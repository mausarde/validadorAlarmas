package validators

import (
	"testing"

	"validadoralarmas/internal/connectors"
)

func TestEvaluarCiclo(t *testing.T) {
	casos := []struct {
		nombre string
		fila   connectors.Fila
		estado string
		etapa  string
	}{
		{"general ok", connectors.Fila{"general": "1"}, CicloOK, ""},
		{"falla en descarga", connectors.Fila{"general": "0", "conexion": "1", "busqueda": "1", "descarga": "0", "volcado": "1"}, CicloNOK, "descarga"},
		{"el primer cero manda", connectors.Fila{"general": "0", "conexion": "1", "busqueda": "1", "descarga": "1", "volcado": "0", "filtrado": "0"}, CicloNOK, "volcado"},
		{"columnas con otro case", connectors.Fila{"GENERAL": "0", "Conexion": "0"}, CicloNOK, "conexion"},
		{"general 0 sin etapa", connectors.Fila{"general": "0", "conexion": "1"}, CicloNOK, ""},
		{"valor desconocido", connectors.Fila{"general": "NULL"}, CicloDesconocido, ""},
		{"sin columna general", connectors.Fila{}, CicloDesconocido, ""},
	}
	for _, c := range casos {
		got := EvaluarCiclo(c.fila)
		if got.Estado != c.estado || got.Etapa != c.etapa {
			t.Errorf("%s: got %+v, esperado estado=%s etapa=%q", c.nombre, got, c.estado, c.etapa)
		}
	}
}
