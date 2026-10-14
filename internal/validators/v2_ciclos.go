package validators

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"validadoralarmas/internal/connectors"
	"validadoralarmas/internal/queries"
)

// Estados posibles de un ciclo.
const (
	CicloOK          = "OK"
	CicloNOK         = "NOK"
	CicloDesconocido = "DESCONOCIDO"
)

// etapasCiclo en el orden de evaluación indicado por la especificación.
var etapasCiclo = []string{"conexion", "busqueda", "descarga", "volcado", "filtrado", "traspaso", "panel"}

// EvaluacionCiclo es la interpretación de una fila de PCA_CICLOS.
type EvaluacionCiclo struct {
	Estado string // CicloOK, CicloNOK o CicloDesconocido
	Etapa  string // primera etapa en 0 (solo si Estado == CicloNOK)
}

// EvaluarCiclo interpreta general (1=OK, 0=NOK) y, si es 0, la primera etapa con valor 0.
func EvaluarCiclo(f connectors.Fila) EvaluacionCiclo {
	g, _ := f.Obtener("general")
	switch strings.TrimSpace(g) {
	case "1":
		return EvaluacionCiclo{Estado: CicloOK}
	case "0":
		for _, e := range etapasCiclo {
			if v, ok := f.Obtener(e); ok && strings.TrimSpace(v) == "0" {
				return EvaluacionCiclo{Estado: CicloNOK, Etapa: e}
			}
		}
		return EvaluacionCiclo{Estado: CicloNOK}
	}
	return EvaluacionCiclo{Estado: CicloDesconocido}
}

// V2Ciclos consulta el último ciclo del Loader por gestor y tipo.
type V2Ciclos struct{}

func (V2Ciclos) Nombre() string { return "Ejecución del loader" }

func (V2Ciclos) Run(ctx context.Context, st *State) Result {
	if st.DBProcess == nil {
		return omitida("dbprocess no disponible (V1 reportó falla de conexión)")
	}
	tipos := []string{"alarma", "cese"}
	gestores := st.Loader.Gestores
	st.DefinirPasos(len(gestores) * len(tipos))
	consulta := fmt.Sprintf(queries.CiclosPorGestorTipo, st.Cfg.Esquema.TablaCiclos)
	tabla := Tabla{Titulo: "Último ciclo por gestor y tipo", Columnas: []string{"Gestor", "Tipo", "Estado", "Etapa fallida", "Fecha del ciclo"}}
	var fallas []string
	var errs []error
	correctos, sinDatos := 0, 0

	for _, g := range gestores {
		for _, tipo := range tipos {
			etiqueta := g.ID + "/" + tipo
			res, err := st.DBProcess.Consultar(ctx, consulta, g.ID, tipo)
			switch {
			case err != nil:
				tabla.Filas = append(tabla.Filas, []string{g.ID, tipo, "ERROR", "-", "-"})
				fallas = append(fallas, etiqueta+": error de consulta ("+resumenErr(err)+")")
				errs = append(errs, err)
			case len(res.Filas) == 0:
				tabla.Filas = append(tabla.Filas, []string{g.ID, tipo, "SIN REGISTROS", "-", "-"})
				sinDatos++
			default:
				ev := EvaluarCiclo(res.Filas[0])
				fecha, ok := res.Filas[0].Obtener(st.Cfg.Esquema.ColFechaCiclo)
				if !ok {
					fecha = "(sin columna '" + st.Cfg.Esquema.ColFechaCiclo + "')"
				}
				switch ev.Estado {
				case CicloOK:
					correctos++
					tabla.Filas = append(tabla.Filas, []string{g.ID, tipo, "OK", "-", fecha})
				case CicloNOK:
					etapa := ev.Etapa
					if etapa == "" {
						etapa = "(no determinable)"
						fallas = append(fallas, etiqueta+": general=0 sin etapa identificable")
					} else {
						fallas = append(fallas, fmt.Sprintf("%s: falló en etapa '%s'", etiqueta, etapa))
					}
					tabla.Filas = append(tabla.Filas, []string{g.ID, tipo, "NOK", etapa, fecha})
				default:
					sinDatos++
					tabla.Filas = append(tabla.Filas, []string{g.ID, tipo, "DESCONOCIDO", "-", fecha})
				}
			}
			st.Paso(etiqueta)
		}
	}

	total := len(gestores) * len(tipos)
	r := Result{Tablas: []Tabla{tabla}, Err: errors.Join(errs...)}
	switch {
	case len(fallas) > 0:
		r.Estado, r.Resumen = NOK, strings.Join(fallas, "; ")
	case sinDatos > 0:
		r.Estado, r.Resumen = Advertencia, fmt.Sprintf("%d de %d ciclos sin registro o con 'general' no interpretable", sinDatos, total)
	default:
		r.Estado, r.Resumen = OK, fmt.Sprintf("%d/%d ciclos con general=1", correctos, total)
	}
	return r
}
