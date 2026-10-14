// Package scanner lista archivos fuente y busca un AID dentro de .tar.gz por streaming.
package scanner

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// ColumnaAID es la columna de la cabecera que contiene el AID en los archivos de alarmas.
	ColumnaAID = "eventPoId"
	separador  = "|"
	maxLinea   = 4 * 1024 * 1024 // tamaño máximo de una línea del archivo
)

// Archivo es un archivo fuente candidato.
type Archivo struct {
	Nombre     string
	Ruta       string
	Modificado time.Time
}

// ListarArchivos devuelve los últimos `limite` archivos de dir que coinciden con patron,
// ordenados por fecha de modificación descendente.
func ListarArchivos(dir, patron string, limite int) ([]Archivo, error) {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el directorio %q: %w", dir, err)
	}
	var res []Archivo
	for _, e := range entradas {
		ok, err := filepath.Match(patron, e.Name())
		if err != nil {
			return nil, fmt.Errorf("patrón %q inválido: %w", patron, err)
		}
		if !ok {
			continue
		}
		ruta := filepath.Join(dir, e.Name())
		info, err := os.Stat(ruta) // sigue enlaces simbólicos
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		res = append(res, Archivo{Nombre: e.Name(), Ruta: ruta, Modificado: info.ModTime()})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Modificado.After(res[j].Modificado) })
	if limite > 0 && len(res) > limite {
		res = res[:limite]
	}
	return res, nil
}

// Coincidencia indica si el AID se halló, en qué miembro del tar y en qué línea (1 = cabecera).
type Coincidencia struct {
	Encontrada bool
	Miembro    string
	Linea      int
}

type lectorCtx struct {
	ctx context.Context
	r   io.Reader
}

func (l *lectorCtx) Read(p []byte) (int, error) {
	if err := l.ctx.Err(); err != nil {
		return 0, err
	}
	return l.r.Read(p)
}

// BuscarEnTarGz lee el .tar.gz en streaming (sin extraer a disco). Cada miembro es un texto
// delimitado por '|' cuya primera línea es la cabecera; el AID debe coincidir exactamente con el
// valor de la columna eventPoId de alguna fila.
func BuscarEnTarGz(ctx context.Context, ruta, aid string) (Coincidencia, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return Coincidencia{}, fmt.Errorf("no se pudo abrir %q: %w", filepath.Base(ruta), err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(&lectorCtx{ctx: ctx, r: f})
	if err != nil {
		return Coincidencia{}, fmt.Errorf("gzip inválido en %q: %w", filepath.Base(ruta), err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return Coincidencia{}, nil
		}
		if err != nil {
			return Coincidencia{}, fmt.Errorf("tar inválido en %q: %w", filepath.Base(ruta), err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		linea, err := buscarEnMiembro(tr, aid)
		if err != nil {
			return Coincidencia{}, fmt.Errorf("leyendo %q en %q: %w", h.Name, filepath.Base(ruta), err)
		}
		if linea > 0 {
			return Coincidencia{Encontrada: true, Miembro: h.Name, Linea: linea}, nil
		}
	}
}

// buscarEnMiembro devuelve el número de línea (base 1) de la primera fila cuyo eventPoId es el AID,
// o 0 si no hay coincidencia. La columna se ubica contando desde el final de la línea, de modo que
// un '|' dentro de campos de texto intermedios (problemText, etc.) no corre las posiciones.
func buscarEnMiembro(r io.Reader, aid string) (int, error) {
	if aid == "" {
		return 0, nil
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLinea)
	if !sc.Scan() {
		return 0, sc.Err()
	}
	cab := strings.Split(strings.TrimRight(sc.Text(), "\r"), separador)
	idx := -1
	for i, c := range cab {
		if strings.TrimSpace(c) == ColumnaAID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, nil // el miembro no tiene la columna eventPoId: no es un archivo de alarmas
	}
	desdeFin := len(cab) - 1 - idx
	patron := []byte(aid)
	n := 1
	for sc.Scan() {
		n++
		if !bytes.Contains(sc.Bytes(), patron) { // filtro rápido sin asignar memoria
			continue
		}
		campos := strings.Split(strings.TrimRight(sc.Text(), "\r"), separador)
		if len(campos) < len(cab) {
			continue
		}
		if strings.TrimSpace(campos[len(campos)-1-desdeFin]) == aid {
			return n, nil
		}
	}
	return 0, sc.Err()
}
