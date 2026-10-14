package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func escribirEnv(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "config")
	contenido := "# comentario\n\nexport enmsvr=\"10.0.0.1\"\nenmport=22\nenmuser='usuario'\n" +
		"enmpass=clave#secreta # nota\nvacia=\n"
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func TestResolverEnvFile(t *testing.T) {
	r := NuevoResolver(escribirEnv(t))
	casos := []struct{ entrada, esperado string }{
		{"ENVFILE->enmsvr", "10.0.0.1"},
		{"ENVFILE->enmport", "22"},
		{"ENVFILE->enmuser", "usuario"},
		{"ENVFILE->enmpass", "clave#secreta"},
		{"ENVFILE->vacia", ""},
		{"5023", "5023"},
	}
	for _, c := range casos {
		got, err := r.Valor(c.entrada)
		if err != nil || got != c.esperado {
			t.Errorf("Valor(%q) = %q, %v; se esperaba %q", c.entrada, got, err, c.esperado)
		}
	}
}

func TestResolverClaveFaltante(t *testing.T) {
	r := NuevoResolver(escribirEnv(t))
	_, err := r.Valor("ENVFILE->noexiste")
	if err == nil || !strings.Contains(err.Error(), "noexiste") {
		t.Fatalf("el error debe nombrar la clave faltante: %v", err)
	}
	for _, sensible := range []string{"10.0.0.1", "clave#secreta", "usuario"} {
		if strings.Contains(err.Error(), sensible) {
			t.Errorf("el error no debe contener valores del ENVFILE: %v", err)
		}
	}
}

func TestResolverArchivoInexistente(t *testing.T) {
	r := NuevoResolver(filepath.Join(t.TempDir(), "no-existe"))
	if _, err := r.Valor("ENVFILE->x"); err == nil {
		t.Fatal("se esperaba error por ENVFILE inexistente")
	}
}

func TestResolverGestorPuertoLiteral(t *testing.T) {
	r := NuevoResolver(escribirEnv(t))
	g := Gestor{ID: "G", Retry: 3, SFTP: SFTP{Server: "ENVFILE->enmsvr", Port: "5023", User: "ENVFILE->enmuser", Password: "ENVFILE->enmpass"}}
	res, err := r.ResolverGestor(g)
	if err != nil || res.HostPuerto() != "10.0.0.1:5023" {
		t.Fatalf("resultado inesperado: %+v, %v", res, err)
	}
}
