package validators

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// V7Panel busca el AID en PCA_PANEL_xG y, si no está, en PCA_HISTORICO_xG.
type V7Panel struct{}

func (V7Panel) Nombre() string { return "Panel" }

func (V7Panel) Run(ctx context.Context, st *State) Result {
	if len(st.Tecnologias) == 0 {
		return omitida("V5 no validó la alarma (sin tecnología asignada)")
	}
	if st.DBFinal == nil {
		return omitida("dbfinal no disponible (V1 reportó falla de conexión)")
	}
	e := st.Cfg.Esquema
	st.DefinirPasos(len(st.Tecnologias))
	var lineas []string
	var errs []error
	r := Result{Estado: OK}

	for _, tec := range st.Tecnologias {
		panel := nombreTabla(e.TablaPanel, tec)
		hist := nombreTabla(e.TablaHistorico, tec)

		resP, errP := buscarAID(ctx, st.DBFinal, st.AID, panel, e.ColAIDPanel)
		if errP == nil && len(resP.Filas) > 0 {
			lineas = append(lineas, fmt.Sprintf("%s: en panel (%s)", tec, panel))
			r.Tablas = append(r.Tablas, tablaRegistro("Registro en "+panel, resP.Columnas, resP.Filas[0], 25))
			st.Paso(panel)
			continue
		}
		if errP != nil {
			errs = append(errs, errP)
			r.Detalles = append(r.Detalles, Detalle{"Advertencia " + tec, "falló la consulta a " + panel + ": " + resumenErr(errP)})
		}

		resH, errH := buscarAID(ctx, st.DBFinal, st.AID, hist, e.ColAIDHistorico)
		switch {
		case errH == nil && len(resH.Filas) > 0:
			f := resH.Filas[0]
			motivo, okM := f.Obtener(e.ColMotivo)
			fin, okF := f.Obtener(e.ColFin)
			if !okM {
				motivo = "(columna " + e.ColMotivo + " no presente)"
			}
			if !okF {
				fin = "(columna " + e.ColFin + " no presente)"
			}
			lineas = append(lineas, fmt.Sprintf("%s: en histórico (%s) %s=%s, %s=%s", tec, hist, e.ColMotivo, motivo, e.ColFin, fin))
			r.Tablas = append(r.Tablas, tablaRegistro("Registro en "+hist, resH.Columnas, f, 25))
		case errH != nil:
			r.Estado = NOK
			errs = append(errs, errH)
			lineas = append(lineas, fmt.Sprintf("%s: error al consultar %s (%s)", tec, hist, resumenErr(errH)))
		default:
			r.Estado = NOK
			lineas = append(lineas, fmt.Sprintf("%s: NO está en %s ni en %s", tec, panel, hist))
		}
		st.Paso(hist)
	}
	r.Resumen = strings.Join(lineas, "; ")
	r.Err = errors.Join(errs...)
	return r
}
