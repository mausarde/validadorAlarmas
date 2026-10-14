package validators

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"validadoralarmas/internal/queries"
)

// V4Volcado busca el AID en PCA_ENM_DATA y lo compara con V3.
type V4Volcado struct{}

func (V4Volcado) Nombre() string { return "Volcado" }

// normalizarArchivo compara por nombre base y sin la extensión .tar.gz. Por definición del proceso
// de extracción, ARCHIVO_FUENTE siempre trae .tar.gz; la normalización solo da tolerancia.
func normalizarArchivo(s string) string {
	return strings.TrimSuffix(filepath.Base(strings.TrimSpace(s)), ".tar.gz")
}

func (V4Volcado) Run(ctx context.Context, st *State) Result {
	if st.DBProcess == nil {
		return omitida("dbprocess no disponible (V1 reportó falla de conexión)")
	}
	st.DefinirPasos(2)
	res, err := st.DBProcess.Consultar(ctx, queries.VolcadoPorAID, st.AID)
	st.Paso("consulta a PCA_ENM_DATA")
	if err != nil {
		return falloErr("no se pudo consultar PCA_ENM_DATA", err)
	}
	st.Volcado = res.Filas

	type clave struct{ gestor, archivo string }
	enArchivos := map[clave]bool{}
	for _, o := range st.Origenes {
		enArchivos[clave{o.Gestor, normalizarArchivo(o.Archivo)}] = true
	}

	tabla := Tabla{
		Titulo:   "Registros en PCA_ENM_DATA",
		Columnas: []string{"GESTOR", "ARCHIVO_FUENTE", "nodeName", "specificProblem", "eventTime", "ceaseTime", "Coincide con V3"},
	}
	enTabla := map[clave]bool{}
	gestorVisto := map[string]bool{}
	reportadas := map[clave]bool{}
	var discrepancias []string
	coincidencias := 0

	for _, f := range res.Filas {
		g, _ := f.Obtener("GESTOR")
		a, _ := f.Obtener("ARCHIVO_FUENTE")
		k := clave{g, normalizarArchivo(a)}
		enTabla[k] = true
		if g != "" && !gestorVisto[g] {
			gestorVisto[g] = true
			st.GestoresVolcado = append(st.GestoresVolcado, g)
		}
		coincide := "No"
		if enArchivos[k] {
			coincide = "Sí"
			coincidencias++
		} else if len(st.Origenes) > 0 && !reportadas[k] {
			reportadas[k] = true
			discrepancias = append(discrepancias,
				fmt.Sprintf("registro con GESTOR=%s y ARCHIVO_FUENTE=%s no corresponde a ninguna coincidencia de V3", g, a))
		}
		nodo, _ := f.Obtener("nodeName")
		sp, _ := f.Obtener("specificProblem")
		et, _ := f.Obtener("eventTime")
		ct, _ := f.Obtener("ceaseTime")
		tabla.Filas = append(tabla.Filas, []string{g, a, nodo, recortar(sp, 60), et, ct, coincide})
	}
	// Coincidencias de V3 que no figuran en la tabla.
	vistos := map[clave]bool{}
	for _, o := range st.Origenes {
		k := clave{o.Gestor, normalizarArchivo(o.Archivo)}
		if !enTabla[k] && !vistos[k] {
			vistos[k] = true
			discrepancias = append(discrepancias,
				fmt.Sprintf("el AID está en el archivo %s (%s/%s) pero no existe en PCA_ENM_DATA para ese archivo", o.Archivo, o.Gestor, o.Tipo))
		}
	}
	st.Paso("comparación con V3")

	r := Result{Tablas: []Tabla{tabla}}
	for _, d := range discrepancias {
		r.Detalles = append(r.Detalles, Detalle{"Discrepancia", d})
	}
	r.Tablas[0].SoloReporte = len(discrepancias) == 0
	n := len(res.Filas)
	switch {
	case n == 0 && len(st.Origenes) > 0:
		r.Estado, r.Resumen = NOK, "el AID está en archivo(s) fuente pero ausente en PCA_ENM_DATA"
	case n == 0:
		r.Estado, r.Resumen = NOK, "el AID no existe en PCA_ENM_DATA"
	case len(st.Origenes) == 0:
		r.Estado = Advertencia
		r.Resumen = fmt.Sprintf("%d registro(s) en PCA_ENM_DATA, pero V3 no localizó el AID en archivos (pudo venir de uno más antiguo)", n)
	case coincidencias == 0:
		r.Estado, r.Resumen = NOK, fmt.Sprintf("%d registro(s) en PCA_ENM_DATA, ninguno coincide con los archivos de V3", n)
	case len(discrepancias) > 0:
		r.Estado = Advertencia
		r.Resumen = fmt.Sprintf("%d registro(s), %d coincidencia(s) con V3 y %d discrepancia(s)", n, coincidencias, len(discrepancias))
	default:
		r.Estado, r.Resumen = OK, fmt.Sprintf("%d registro(s) en PCA_ENM_DATA, coinciden con V3", n)
	}
	return r
}
