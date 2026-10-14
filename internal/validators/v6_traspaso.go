package validators

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// V6Traspaso busca el AID en dbfinal, tabla PCA_ALARMAS_xG según la tecnología de V5.
type V6Traspaso struct{}

func (V6Traspaso) Nombre() string { return "Traspaso" }

func (V6Traspaso) Run(ctx context.Context, st *State) Result {
	if len(st.Tecnologias) == 0 {
		return omitida("V5 no validó la alarma (sin tecnología asignada)")
	}
	if st.DBFinal == nil {
		return omitida("dbfinal no disponible (V1 reportó falla de conexión)")
	}
	st.DefinirPasos(len(st.Tecnologias))
	var lineas []string
	var errs []error
	r := Result{Estado: OK}
	for _, tec := range st.Tecnologias {
		tabla := nombreTabla(st.Cfg.Esquema.TablaAlarmas, tec)
		res, err := buscarAID(ctx, st.DBFinal, st.AID, tabla, st.Cfg.Esquema.ColAIDAlarmas)
		switch {
		case err != nil:
			r.Estado = NOK
			errs = append(errs, err)
			lineas = append(lineas, fmt.Sprintf("%s: error al consultar %s (%s)", tec, tabla, resumenErr(err)))
		case len(res.Filas) == 0:
			r.Estado = NOK
			lineas = append(lineas, fmt.Sprintf("%s: NO encontrada en %s", tec, tabla))
		default:
			lineas = append(lineas, fmt.Sprintf("%s: encontrada en %s", tec, tabla))
			r.Tablas = append(r.Tablas, tablaRegistro("Registro en "+tabla, res.Columnas, res.Filas[0], 25))
		}
		st.Paso(tabla)
	}
	r.Resumen = strings.Join(lineas, "; ")
	r.Err = errors.Join(errs...)
	return r
}
