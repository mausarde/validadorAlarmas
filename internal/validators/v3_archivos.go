package validators

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"validadoralarmas/internal/scanner"
)

// V3Archivos busca el AID en los últimos N .tar.gz de readyPath por gestor y tipo.
type V3Archivos struct{}

func (V3Archivos) Nombre() string { return "Archivos fuente" }

func (V3Archivos) Run(ctx context.Context, st *State) Result {
	ruta, err := st.Res.Valor(st.Loader.ReadyPath)
	if err != nil {
		return falloErr("no se pudo resolver readyPath", err)
	}
	type tarea struct {
		gestor, tipo string
		arch         scanner.Archivo
	}
	var tareas []tarea
	var detalles []Detalle
	for _, g := range st.Loader.Gestores {
		for _, tp := range []struct{ tipo, patron string }{{"alarma", g.FilesAlarmPattern}, {"cese", g.FilesClearPattern}} {
			archivos, err := scanner.ListarArchivos(ruta, tp.patron, st.Cfg.MaxArchivosPorGestorTipo)
			if err != nil {
				return falloErr("no se pudo listar los archivos de readyPath", err)
			}
			if len(archivos) == 0 {
				detalles = append(detalles, Detalle{g.ID + "/" + tp.tipo, "sin archivos que coincidan con '" + tp.patron + "' en readyPath"})
			}
			for _, a := range archivos {
				tareas = append(tareas, tarea{g.ID, tp.tipo, a})
			}
		}
	}
	st.DefinirPasos(max(len(tareas), 1))

	tabla := Tabla{Titulo: "Archivos revisados", Columnas: []string{"Gestor", "Tipo", "Archivo", "Modificado", "Resultado"}}
	var errs []error
	errores := 0
	for _, t := range tareas {
		if ctx.Err() != nil {
			break
		}
		st.Log.Trace("leyendo archivo", "archivo", t.arch.Nombre, "gestor", t.gestor, "tipo", t.tipo)
		c, err := scanner.BuscarEnTarGz(ctx, t.arch.Ruta, st.AID)
		resultado := "no"
		switch {
		case err != nil:
			errores++
			errs = append(errs, err)
			resultado = "error: " + resumenErr(err)
		case c.Encontrada:
			resultado = fmt.Sprintf("COINCIDE (%s, línea %d)", c.Miembro, c.Linea)
			st.Origenes = append(st.Origenes, Origen{Gestor: t.gestor, Tipo: t.tipo, Archivo: t.arch.Nombre, Miembro: c.Miembro})
		}
		tabla.Filas = append(tabla.Filas, []string{t.gestor, t.tipo, t.arch.Nombre, t.arch.Modificado.Format("2006-01-02 15:04:05"), resultado})
		st.Paso(t.arch.Nombre)
	}

	r := Result{Err: errors.Join(errs...), Detalles: detalles}
	if len(st.Origenes) > 0 {
		o := st.Origenes[0]
		r.Estado = OK
		r.Resumen = fmt.Sprintf("%s | %s | %s", o.Archivo, o.Gestor, o.Tipo)
		if len(st.Origenes) > 1 {
			r.Resumen += fmt.Sprintf(" (+%d más)", len(st.Origenes)-1)
		}
		for i, o := range st.Origenes {
			r.Detalles = append(r.Detalles, Detalle{fmt.Sprintf("Coincidencia %d", i+1), fmt.Sprintf("%s | %s | %s (miembro %s)", o.Archivo, o.Gestor, o.Tipo, o.Miembro)})
		}
		if errores > 0 {
			r.Detalles = append(r.Detalles, Detalle{"Advertencia", fmt.Sprintf("%d archivo(s) no pudieron leerse", errores)})
		}
		tabla.SoloReporte = errores == 0
	} else {
		r.Estado = NOK
		r.Resumen = fmt.Sprintf("AID no encontrado en %d archivo(s) revisados", len(tareas)-errores)
		if errores > 0 {
			r.Resumen += fmt.Sprintf(" (%d con error de lectura)", errores)
		}
		if ctx.Err() != nil {
			r.Resumen += " (búsqueda interrumpida)"
		}
		r.Detalles = append(r.Detalles, Detalle{"Nota", fmt.Sprintf("el AID se busca solo en los últimos %d archivos por gestor y tipo; puede estar en archivos más antiguos: aumente maxArchivosPorGestorTipo", st.Cfg.MaxArchivosPorGestorTipo)})
		r.Detalles = append(r.Detalles, Detalle{"Archivos revisados", archivosRevisados(tabla)})
	}
	r.Tablas = []Tabla{tabla}
	return r
}

func archivosRevisados(t Tabla) string {
	if len(t.Filas) == 0 {
		return "ninguno"
	}
	nombres := make([]string, 0, len(t.Filas))
	for _, f := range t.Filas {
		nombres = append(nombres, f[0]+"/"+f[1]+": "+f[2])
	}
	return recortar(strings.Join(nombres, "; "), 600)
}
