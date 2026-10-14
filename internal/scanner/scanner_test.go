package scanner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func crearTarGz(t *testing.T, ruta string, miembros [][2]string) {
	t.Helper()
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, m := range miembros {
		h := &tar.Header{Name: m[0], Mode: 0o600, Size: int64(len(m[1])), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(m[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

const cabecera = "presentSeverity|NodeName|alarmId|eventPoId|repeatCount\n"

func TestBuscarEnTarGz(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "file_2026-10-13_11_50_01_789863.tar.gz")
	crearTarGz(t, ruta, [][2]string{
		{"a.txt", cabecera + "MAJOR|N1|555|111|1\n"},
		{"b.txt", cabecera + "MINOR|N2|777|123456789|2\n"},
	})
	ctx := context.Background()

	c, err := BuscarEnTarGz(ctx, ruta, "123456789")
	if err != nil || !c.Encontrada || c.Miembro != "b.txt" || c.Linea != 2 {
		t.Fatalf("resultado inesperado: %+v, %v", c, err)
	}
	// Prefijo de otro id: no debe coincidir.
	if c, err = BuscarEnTarGz(ctx, ruta, "12345678"); err != nil || c.Encontrada {
		t.Fatalf("no debía coincidir por prefijo: %+v, %v", c, err)
	}
	// El valor existe en otra columna (alarmId), no en eventPoId: no debe coincidir.
	if c, err = BuscarEnTarGz(ctx, ruta, "555"); err != nil || c.Encontrada {
		t.Fatalf("no debía coincidir con alarmId: %+v, %v", c, err)
	}
	if _, err := BuscarEnTarGz(ctx, filepath.Join(dir, "no-existe.tar.gz"), "1"); err == nil {
		t.Fatal("se esperaba error por archivo inexistente")
	}
}

func TestBuscarEnMiembro(t *testing.T) {
	casos := []struct {
		nombre, texto string
		esperado      int
	}{
		{"coincide", cabecera + "A|N|1|999|1\n", 2},
		{"AID en otra columna", cabecera + "A|N|999|1|1\n", 0},
		{"pipe dentro de un campo de texto", cabecera + "A|N con | pipe|1|999|1\n", 2},
		{"fila con menos campos se ignora", cabecera + "A|999\n", 0},
		{"fin de línea CRLF", "presentSeverity|NodeName|alarmId|eventPoId|repeatCount\r\nA|N|1|999|1\r\n", 2},
		{"sin columna eventPoId", "a|b|c\nx|999|y\n", 0},
		{"vacío", "", 0},
		{"segunda fila", cabecera + "A|N|1|1|1\nA|N|2|999|1\n", 3},
	}
	for _, c := range casos {
		got, err := buscarEnMiembro(strings.NewReader(c.texto), "999")
		if err != nil || got != c.esperado {
			t.Errorf("%s: got=%d err=%v, esperado %d", c.nombre, got, err, c.esperado)
		}
	}
}

func TestListarArchivosOrdenYLimite(t *testing.T) {
	dir := t.TempDir()
	ahora := time.Now()
	for nombre, antig := range map[string]time.Duration{"a.tar.gz": 3 * time.Hour, "b.tar.gz": time.Hour, "c.tar.gz": 2 * time.Hour} {
		ruta := filepath.Join(dir, nombre)
		if err := os.WriteFile(ruta, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(ruta, ahora.Add(-antig), ahora.Add(-antig)); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "otro.txt"), []byte("x"), 0o600)
	res, err := ListarArchivos(dir, "*.tar.gz", 2)
	if err != nil || len(res) != 2 || res[0].Nombre != "b.tar.gz" || res[1].Nombre != "c.tar.gz" {
		t.Fatalf("resultado inesperado: %+v, %v", res, err)
	}
}
