package validators

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"validadoralarmas/internal/connectors"
	"validadoralarmas/internal/queries"
)

// V5Filtros ejecuta las consultas 3G/4G/5G con el filtro por AID y, si no hay fila, diagnostica por descarte.
type V5Filtros struct{}

func (V5Filtros) Nombre() string { return "Filtros" }

type veredicto struct {
	valida   bool
	noAplica bool // la alarma no pertenece a la tecnología
	linea    string
	detalles []Detalle
	err      error
}

func (V5Filtros) Run(ctx context.Context, st *State) Result {
	if st.DBProcess == nil {
		return omitida("dbprocess no disponible (V1 reportó falla de conexión)")
	}
	if len(st.GestoresVolcado) == 0 {
		return omitida("V4 no halló registros del AID en PCA_ENM_DATA")
	}
	planes := queries.Planes()
	st.DefinirPasos(len(st.GestoresVolcado) * len(planes))

	var lineas []string
	var detalles []Detalle
	var errs []error
	validas := map[string]bool{}
	evaluadas, noAplican := 0, 0

externo:
	for _, g := range st.GestoresVolcado {
		for _, p := range planes {
			if ctx.Err() != nil {
				lineas = append(lineas, "evaluación interrumpida")
				break externo
			}
			v := evaluarPlan(ctx, st, p, g)
			etiqueta := p.Etiqueta
			if len(st.GestoresVolcado) > 1 {
				etiqueta = g + "/" + p.Etiqueta
			}
			lineas = append(lineas, etiqueta+": "+v.linea)
			detalles = append(detalles, v.detalles...)
			evaluadas++
			switch {
			case v.err != nil:
				errs = append(errs, v.err)
			case v.valida:
				validas[p.Tecnologia] = true
			case v.noAplica:
				noAplican++
			}
			st.Paso(etiqueta)
		}
	}

	for _, p := range planes { // orden estable 3G, 4G, 5G
		if validas[p.Tecnologia] {
			st.Tecnologias = append(st.Tecnologias, p.Tecnologia)
		}
	}
	r := Result{Resumen: strings.Join(lineas, "\n"), Detalles: detalles, Err: errors.Join(errs...)}
	if len(st.Tecnologias) > 0 {
		r.Estado = OK
		return r
	}
	r.Estado = NOK
	if evaluadas > 0 && noAplican == evaluadas {
		r.Detalles = append(r.Detalles, Detalle{"Nota", "la alarma no es candidata a ninguna tecnología (3G, 4G ni 5G)"})
	}
	return r
}

func evaluarPlan(ctx context.Context, st *State, p queries.Plan, gestor string) veredicto {
	res, err := st.DBProcess.Consultar(ctx, p.Consulta, gestor, st.AID)
	if err != nil {
		return veredicto{err: err, linea: "no se pudo evaluar (" + resumenErr(err) + ")"}
	}
	if len(res.Filas) > 0 {
		return validaPlan(p, res.Filas[0])
	}
	return diagnosticarPlan(ctx, st, p, gestor)
}

func validaPlan(p queries.Plan, f connectors.Fila) veredicto {
	get := func(k string) string { v, _ := f.Obtener(k); return v }
	var cumple []string
	for _, c := range p.Pasos() {
		cumple = append(cumple, c.Pasa)
	}
	linea := fmt.Sprintf("VÁLIDA: %s. CIRCUIT=%s, SITE=%s, clase=%s, subtype=%s.",
		strings.Join(cumple, "; "), get("CIRCUIT"), get("SITE"), get("clase"), get("subtype"))
	campos := []string{"NW", "NE", "EQ", "clase", "type", "subtype", "CIRCUIT", "SITE", "tecnologia", "inicio", "fin", "ack", "GESTOR", "FUENTE"}
	var partes []string
	for _, c := range campos {
		if v, ok := f.Obtener(c); ok {
			partes = append(partes, c+"="+recortar(v, 80))
		}
	}
	return veredicto{
		valida: true, linea: linea,
		detalles: []Detalle{{p.Tecnologia + " campos calculados", strings.Join(partes, ", ")}},
	}
}

func contar(ctx context.Context, st *State, consulta, gestor string) (int, error) {
	res, err := st.DBProcess.Consultar(ctx, consulta, gestor, st.AID)
	if err != nil {
		return 0, err
	}
	if len(res.Filas) == 0 {
		return 0, errors.New("la consulta de conteo no devolvió filas")
	}
	v, _ := res.Filas[0].Obtener("n")
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("conteo no numérico: %w", err)
	}
	return n, nil
}

// valoresCausa muestra los valores del registro que explican la exclusión.
func valoresCausa(f connectors.Fila, tieneFila bool, campos []string) string {
	if !tieneFila {
		return ""
	}
	var partes []string
	for _, c := range campos {
		if v, ok := f.Obtener(c); ok {
			partes = append(partes, fmt.Sprintf("%s='%s'", c, recortar(v, 120)))
		} else {
			partes = append(partes, c+": columna inexistente en PCA_ENM_DATA (suposición a confirmar)")
		}
	}
	return strings.Join(partes, ", ")
}

// diagnosticarPlan evalúa las condiciones de a una, en orden: join, pertenencia y exclusiones.
func diagnosticarPlan(ctx context.Context, st *State, p queries.Plan, gestor string) veredicto {
	ev, errEv := st.DBProcess.Consultar(ctx, queries.EvidenciaFila, gestor, st.AID)
	var fila connectors.Fila
	tieneFila := errEv == nil && len(ev.Filas) > 0
	if tieneFila {
		fila = ev.Filas[0]
	}
	pasos := p.Pasos()
	pertenece, err := contar(ctx, st, p.SQLPertenencia(), gestor)
	if err != nil {
		return veredicto{err: err, linea: "no se pudo evaluar la pertenencia (" + resumenErr(err) + ")"}
	}
	idx := -1
	for i := range pasos {
		n, err := contar(ctx, st, p.SQLPaso(i), gestor)
		if err != nil {
			return veredicto{err: err, linea: "no se pudo evaluar '" + pasos[i].ID + "' (" + resumenErr(err) + ")"}
		}
		if n == 0 {
			idx = i
			break
		}
	}
	if pertenece == 0 {
		linea := "no aplica (" + p.Condiciones[0].Falla + ")"
		if v := valoresCausa(fila, tieneFila, p.Condiciones[0].Campos[:1]); v != "" {
			linea += " [" + v + "]"
		}
		if idx == 0 {
			linea += "; además " + p.CondJoin.Falla
		}
		return veredicto{noAplica: true, linea: linea}
	}
	if idx < 0 {
		return veredicto{linea: "no se pudo determinar la causa: cada condición, evaluada por separado, se cumple"}
	}
	c := pasos[idx]
	linea := "excluida (" + c.Falla + ")"
	if v := valoresCausa(fila, tieneFila, c.Campos); v != "" {
		linea += " [" + v + "]"
	}
	return veredicto{linea: linea}
}
