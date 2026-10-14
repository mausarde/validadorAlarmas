// Package report genera el reporte final por pantalla, en .txt y en .md.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"validadoralarmas/internal/logger"
	"validadoralarmas/internal/validators"
)

// Datos agrupa lo necesario para renderizar el reporte.
type Datos struct {
	AID        string
	Inicio     time.Time
	Zona       string
	Nombres    []string
	Resultados []validators.Result
	Parcial    bool
}

const (
	tituloReporte  = "================ REPORTE ValidadorAlarmas ================"
	anchoEtiqueta  = 32
	anchoCeldaPant = 60
)

// NombreZona describe la zona horaria local del servidor.
func NombreZona(t time.Time) string {
	abrev, _ := t.Zone()
	desfase := t.Format("-07:00")
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" {
		return fmt.Sprintf("%s, %s UTC%s", tz, abrev, desfase)
	}
	return fmt.Sprintf("%s UTC%s", abrev, desfase)
}

// NombreArchivo construye el nombre del reporte. Acepta {AID}/{FECHA_HORA} y [AID]/[FECHA_HORA].
// FECHA_HORA usa el formato YYYYMMDD_HHMMSS. Los separadores de ruta se reemplazan por '_'.
func NombreArchivo(patron, aid string, t time.Time, formato string) string {
	if strings.TrimSpace(patron) == "" {
		patron = "{AID}_{FECHA_HORA}"
	}
	fh := t.Format("20060102_150405")
	n := strings.NewReplacer("{AID}", aid, "[AID]", aid, "{FECHA_HORA}", fh, "[FECHA_HORA]", fh).Replace(patron)
	n = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, n)
	ext := ".md"
	if strings.EqualFold(strings.TrimSpace(formato), "txt") {
		ext = ".txt"
	}
	return n + ext
}

// Escribir genera el archivo del reporte (enmascarando secretos) y devuelve su ruta.
func Escribir(d Datos, dir, patron, formato string, masc *logger.Enmascarador) (string, error) {
	var contenido string
	if strings.EqualFold(formato, "txt") {
		contenido = RenderTexto(d, false)
	} else {
		contenido = RenderMarkdown(d)
	}
	if masc != nil {
		contenido = masc.Ocultar(contenido)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("no se pudo crear reportsPath %q: %w", dir, err)
	}
	ruta := filepath.Join(dir, NombreArchivo(patron, d.AID, d.Inicio, formato))
	if err := os.WriteFile(ruta, []byte(contenido), 0o640); err != nil {
		return "", fmt.Errorf("no se pudo escribir el reporte %q: %w", ruta, err)
	}
	return ruta, nil
}

// ---------- Conclusión ----------

func resultado(d Datos, i int) validators.Result {
	if i < len(d.Resultados) {
		return d.Resultados[i]
	}
	return validators.Result{Estado: validators.Omitida, Resumen: "no ejecutada"}
}

// Conclusion devuelve la etapa donde se detuvo la alarma (o "Flujo completo OK") y observaciones.
func Conclusion(d Datos) (string, []string) {
	var obs []string
	est := func(i int) validators.Estado { return resultado(d, i).Estado }

	if est(0) == validators.NOK {
		obs = append(obs, "V1: "+resultado(d, 0).Resumen)
	}
	if e := est(1); e == validators.NOK || e == validators.Advertencia {
		obs = append(obs, "V2: "+resultado(d, 1).Resumen)
	}
	v3Suelto := est(2) == validators.NOK && (est(3) == validators.OK || est(3) == validators.Advertencia)
	if v3Suelto {
		obs = append(obs, "V3: el AID no se localizó en los archivos revisados aunque sí está en PCA_ENM_DATA; "+
			"pudo venir de un archivo más antiguo (aumente maxArchivosPorGestorTipo).")
	}

	principal := ""
	for i := 2; i <= 6 && principal == ""; i++ {
		if est(i) != validators.NOK || (i == 2 && v3Suelto) {
			continue
		}
		switch i {
		case 2:
			if est(3) == validators.NOK {
				principal = "La alarma no llegó: no está en los archivos fuente revisados ni en PCA_ENM_DATA (V3/V4)."
			} else {
				principal = "La alarma no se encontró en los archivos fuente revisados (V3)."
			}
		case 3:
			if est(2) == validators.OK {
				principal = "La alarma llegó en un archivo fuente pero no fue volcada a PCA_ENM_DATA (V4)."
			} else {
				principal = "La alarma no existe en PCA_ENM_DATA (V4)."
			}
		case 4:
			principal = "La alarma llegó y fue volcada, pero fue descartada en el filtrado (V5)."
		case 5:
			principal = "La alarma pasó los filtros pero no fue traspasada a dbfinal (V6)."
		case 6:
			principal = "La alarma fue traspasada pero no aparece en panel ni en histórico (V7)."
		}
	}
	if principal == "" {
		for i := 2; i <= 6 && principal == ""; i++ {
			if est(i) == validators.Omitida {
				principal = fmt.Sprintf("Diagnóstico incompleto: V%d omitida (%s).", i+1, resultado(d, i).Resumen)
			}
		}
	}
	if principal == "" {
		principal = "Flujo completo OK: la alarma recorrió todas las etapas."
	}
	if d.Parcial {
		principal = "REPORTE PARCIAL (ejecución interrumpida). " + principal
	}
	return principal, obs
}

// ---------- Texto (pantalla y .txt) ----------

func nombreValidacion(d Datos, i int) string {
	if i < len(d.Nombres) {
		return d.Nombres[i]
	}
	return fmt.Sprintf("Validación %d", i+1)
}

func prefijo(i int, nombre string, e validators.Estado) string {
	et := fmt.Sprintf("[V%d] %s ", i+1, nombre)
	if n := utf8.RuneCountInString(et); n < anchoEtiqueta {
		et += strings.Repeat(".", anchoEtiqueta-n)
	}
	return fmt.Sprintf("%s %-11s ", et, string(e))
}

func cortar(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func rellenar(s string, ancho int) string {
	if n := utf8.RuneCountInString(s); n < ancho {
		return s + strings.Repeat(" ", ancho-n)
	}
	return s
}

func tablaTexto(t validators.Tabla, sangria string, pantalla bool) string {
	limite := 0
	if pantalla {
		limite = anchoCeldaPant
	}
	filas := make([][]string, 0, len(t.Filas)+1)
	filas = append(filas, t.Columnas)
	for _, f := range t.Filas {
		filas = append(filas, f)
	}
	anchos := make([]int, len(t.Columnas))
	for _, f := range filas {
		for j := range t.Columnas {
			c := ""
			if j < len(f) {
				c = cortar(f[j], limite)
			}
			if n := utf8.RuneCountInString(c); n > anchos[j] {
				anchos[j] = n
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n", sangria, t.Titulo)
	for i, f := range filas {
		b.WriteString(sangria + "  ")
		for j := range t.Columnas {
			c := ""
			if j < len(f) {
				c = cortar(f[j], limite)
			}
			b.WriteString(rellenar(c, anchos[j]) + "  ")
		}
		b.WriteString("\n")
		if i == 0 {
			b.WriteString(sangria + "  ")
			for j := range t.Columnas {
				b.WriteString(strings.Repeat("-", anchos[j]) + "  ")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// RenderTexto genera el reporte en texto. En pantalla se omiten las tablas marcadas SoloReporte.
func RenderTexto(d Datos, pantalla bool) string {
	var b strings.Builder
	b.WriteString(tituloReporte + "\n")
	if d.Parcial {
		b.WriteString("*** REPORTE PARCIAL: la ejecución fue interrumpida ***\n")
	}
	fmt.Fprintf(&b, "AID: %s        Ejecución: %s (%s)\n\n", d.AID, d.Inicio.Format("2006-01-02 15:04:05"), d.Zona)

	for i, r := range d.Resultados {
		pre := prefijo(i, nombreValidacion(d, i), r.Estado)
		sangria := strings.Repeat(" ", utf8.RuneCountInString(pre))
		for j, linea := range strings.Split(r.Resumen, "\n") {
			if j == 0 {
				b.WriteString(pre + linea + "\n")
			} else {
				b.WriteString(sangria + linea + "\n")
			}
		}
		for _, dt := range r.Detalles {
			fmt.Fprintf(&b, "%s- %s: %s\n", sangria, dt.Clave, dt.Valor)
		}
		if !pantalla {
			if r.Duracion > 0 {
				fmt.Fprintf(&b, "%s- Duración: %s\n", sangria, r.Duracion.Round(time.Millisecond))
			}
			if r.Err != nil {
				fmt.Fprintf(&b, "%s- Error (resumido): %s\n", sangria, cortar(r.Err.Error(), 300))
			}
		}
		for _, t := range r.Tablas {
			if pantalla && t.SoloReporte {
				continue
			}
			b.WriteString(tablaTexto(t, sangria, pantalla))
		}
	}
	principal, obs := Conclusion(d)
	b.WriteString("\nCONCLUSIÓN: " + principal + "\n")
	for _, o := range obs {
		b.WriteString("  * " + o + "\n")
	}
	b.WriteString(strings.Repeat("=", utf8.RuneCountInString(tituloReporte)) + "\n")
	return b.String()
}

// ---------- Markdown ----------

func mdEsc(s string) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

func tablaMD(b *strings.Builder, t validators.Tabla) {
	fmt.Fprintf(b, "**%s**\n\n|", t.Titulo)
	for _, c := range t.Columnas {
		b.WriteString(" " + mdEsc(c) + " |")
	}
	b.WriteString("\n|")
	for range t.Columnas {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, f := range t.Filas {
		b.WriteString("|")
		for j := range t.Columnas {
			c := ""
			if j < len(f) {
				c = f[j]
			}
			b.WriteString(" " + mdEsc(c) + " |")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// RenderMarkdown genera el reporte con tablas y una sección por validación.
func RenderMarkdown(d Datos) string {
	var b strings.Builder
	b.WriteString("# Reporte ValidadorAlarmas\n\n")
	fmt.Fprintf(&b, "- **AID:** `%s`\n- **Ejecución:** %s (%s)\n\n", d.AID, d.Inicio.Format("2006-01-02 15:04:05"), d.Zona)
	if d.Parcial {
		b.WriteString("> **REPORTE PARCIAL:** la ejecución fue interrumpida.\n\n")
	}
	b.WriteString("## Resumen\n\n| Validación | Estado | Detalle |\n| --- | --- | --- |\n")
	for i, r := range d.Resultados {
		fmt.Fprintf(&b, "| V%d %s | %s | %s |\n", i+1, mdEsc(nombreValidacion(d, i)), r.Estado, mdEsc(r.Resumen))
	}
	b.WriteString("\n")
	for i, r := range d.Resultados {
		fmt.Fprintf(&b, "## V%d — %s\n\n**Estado:** %s\n\n", i+1, nombreValidacion(d, i), r.Estado)
		for _, l := range strings.Split(r.Resumen, "\n") {
			b.WriteString("- " + l + "\n")
		}
		b.WriteString("\n")
		for _, dt := range r.Detalles {
			fmt.Fprintf(&b, "- **%s:** %s\n", mdEsc(dt.Clave), mdEsc(dt.Valor))
		}
		if r.Duracion > 0 {
			fmt.Fprintf(&b, "- **Duración:** %s\n", r.Duracion.Round(time.Millisecond))
		}
		if r.Err != nil {
			fmt.Fprintf(&b, "- **Error (resumido):** %s\n", mdEsc(cortar(r.Err.Error(), 300)))
		}
		b.WriteString("\n")
		for _, t := range r.Tablas {
			tablaMD(&b, t)
		}
	}
	principal, obs := Conclusion(d)
	b.WriteString("## Conclusión\n\n**" + principal + "**\n\n")
	for _, o := range obs {
		b.WriteString("- " + o + "\n")
	}
	return b.String()
}
