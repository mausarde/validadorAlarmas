package report

import (
	"strings"
	"testing"
	"time"

	"validadoralarmas/internal/validators"
)

func TestNombreArchivo(t *testing.T) {
	f := time.Date(2025, 1, 15, 14, 32, 10, 0, time.UTC)
	casos := []struct{ patron, formato, esperado string }{
		{"{AID}_{FECHA_HORA}", "md", "123456789_20250115_143210.md"},
		{"", "md", "123456789_20250115_143210.md"},
		{"[AID]_[FECHA_HORA]", "txt", "123456789_20250115_143210.txt"},
		{"rep/{AID}", "md", "rep_123456789.md"},
	}
	for _, c := range casos {
		if got := NombreArchivo(c.patron, "123456789", f, c.formato); got != c.esperado {
			t.Errorf("NombreArchivo(%q,%q) = %q; esperado %q", c.patron, c.formato, got, c.esperado)
		}
	}
}

func resultados(estados ...validators.Estado) []validators.Result {
	rs := make([]validators.Result, len(estados))
	for i, e := range estados {
		rs[i] = validators.Result{Estado: e, Resumen: "x"}
	}
	return rs
}

func TestConclusion(t *testing.T) {
	ok, nok, om := validators.OK, validators.NOK, validators.Omitida
	casos := []struct {
		nombre   string
		estados  []validators.Estado
		contiene string
	}{
		{"flujo completo", []validators.Estado{ok, ok, ok, ok, ok, ok, ok}, "Flujo completo OK"},
		{"descartada en filtrado", []validators.Estado{ok, ok, ok, ok, nok, om, om}, "(V5)"},
		{"no volcada", []validators.Estado{ok, ok, ok, nok, om, om, om}, "(V4)"},
		{"sin panel", []validators.Estado{ok, ok, ok, ok, ok, ok, nok}, "(V7)"},
		{"V3 suelto pero V4 ok", []validators.Estado{ok, ok, nok, ok, ok, ok, ok}, "Flujo completo OK"},
		{"incompleto", []validators.Estado{nok, ok, ok, om, om, om, om}, "Diagnóstico incompleto"},
	}
	for _, c := range casos {
		got, _ := Conclusion(Datos{Resultados: resultados(c.estados...)})
		if !strings.Contains(got, c.contiene) {
			t.Errorf("%s: %q no contiene %q", c.nombre, got, c.contiene)
		}
	}
}
