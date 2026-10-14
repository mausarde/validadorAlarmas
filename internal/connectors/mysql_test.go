package connectors

import "testing"

func TestSoportaTxSoloLectura(t *testing.T) {
	casos := []struct {
		version  string
		esperado bool
	}{
		{"5.5.56-MariaDB", false},                                 // dbprocess
		{"5.7.21-enterprise-commercial-advanced-log", true},       // dbfinal
		{"5.6.4-log", false},
		{"5.6.5", true},
		{"8.0.35", true},
		{"10.0.38-MariaDB", true},
		{"10.3.22-MariaDB-1:10.3.22+maria~bionic", true},
		{"5.5.5-10.3.22-MariaDB", true}, // prefijo de compatibilidad de replicación
		{"5.5.5-10.11.6-MariaDB-log", true},
		{"", false},
		{"desconocida", false},
	}
	for _, c := range casos {
		if got := SoportaTxSoloLectura(c.version); got != c.esperado {
			t.Errorf("SoportaTxSoloLectura(%q) = %v; esperado %v", c.version, got, c.esperado)
		}
	}
}

func TestEsSelect(t *testing.T) {
	for consulta, esperado := range map[string]bool{
		"SELECT 1":                 true,
		"  select * from t":        true,
		"\n\tSELECT DISTINCT a":    true,
		"UPDATE t SET a=1":         false,
		"DELETE FROM t":            false,
		"INSERT INTO t VALUES (1)": false,
		"":                         false,
	} {
		if got := esSelect(consulta); got != esperado {
			t.Errorf("esSelect(%q) = %v; esperado %v", consulta, got, esperado)
		}
	}
}
