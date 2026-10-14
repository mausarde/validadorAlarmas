package queries

import (
	"strings"
	"testing"
)

func TestPlanesParametros(t *testing.T) {
	for _, p := range Planes() {
		if n := strings.Count(p.Consulta, "?"); n != 2 {
			t.Errorf("%s: la consulta base debe tener 2 parámetros (gestor, AID), tiene %d", p.Tecnologia, n)
		}
		if strings.Contains(p.Consulta, "`") {
			t.Errorf("%s: la consulta no debe contener comillas invertidas", p.Tecnologia)
		}
		for i := range p.Pasos() {
			if n := strings.Count(p.SQLPaso(i), "?"); n != 2 {
				t.Errorf("%s paso %d: se esperaban 2 parámetros, hay %d", p.Tecnologia, i, n)
			}
		}
		if n := strings.Count(p.SQLPertenencia(), "?"); n != 2 {
			t.Errorf("%s: SQLPertenencia debe tener 2 parámetros", p.Tecnologia)
		}
	}
}
