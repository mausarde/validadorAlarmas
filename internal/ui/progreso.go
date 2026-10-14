package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/term"
)

// Progreso es la vista de avance: barra general (N de 7) y barra de la validación en curso.
type Progreso interface {
	IniciarValidacion(n int, nombre string)
	DefinirPasos(total int)
	Paso(descripcion string)
	FinValidacion(estado string)
	Cerrar()
}

// NuevoProgreso usa barras si salida es una terminal; si no, líneas de texto simples.
func NuevoProgreso(salida *os.File, total int) Progreso {
	if term.IsTerminal(int(salida.Fd())) {
		return nuevoMPB(salida, total)
	}
	return &barrasTexto{out: salida, total: total}
}

func recortarTexto(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ---------- Implementación con mpb ----------

type barrasMPB struct {
	p       *mpb.Progress
	general *mpb.Bar
	actual  *mpb.Bar
	mu      sync.Mutex
	paso    string
}

func nuevoMPB(out io.Writer, total int) *barrasMPB {
	p := mpb.New(mpb.WithOutput(out), mpb.WithWidth(40), mpb.WithRefreshRate(100*time.Millisecond))
	g := p.AddBar(int64(total),
		mpb.PrependDecorators(decor.Name("General                        "), decor.CountersNoUnit("%d/%d ")),
		mpb.AppendDecorators(decor.Percentage()),
	)
	return &barrasMPB{p: p, general: g}
}

func (b *barrasMPB) IniciarValidacion(n int, nombre string) {
	b.mu.Lock()
	b.paso = ""
	b.mu.Unlock()
	titulo := fmt.Sprintf("%-30.30s", fmt.Sprintf("V%d %s", n, nombre))
	b.actual = b.p.AddBar(1,
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(decor.Name(titulo+" "), decor.CountersNoUnit("%d/%d ")),
		mpb.AppendDecorators(decor.Any(func(decor.Statistics) string {
			b.mu.Lock()
			defer b.mu.Unlock()
			return recortarTexto(b.paso, 40)
		})),
	)
}

func (b *barrasMPB) DefinirPasos(total int) {
	if b.actual == nil {
		return
	}
	b.actual.SetTotal(int64(max(total, 1)), false)
}

func (b *barrasMPB) Paso(descripcion string) {
	b.mu.Lock()
	b.paso = descripcion
	b.mu.Unlock()
	if b.actual != nil {
		b.actual.Increment()
	}
}

func (b *barrasMPB) FinValidacion(string) {
	if b.actual != nil {
		b.actual.SetTotal(-1, true)
		b.actual = nil
	}
	b.general.Increment()
}

func (b *barrasMPB) Cerrar() {
	if b.actual != nil {
		b.actual.SetTotal(-1, true) // evita que Wait() se bloquee ante una interrupción
	}
	b.general.SetTotal(-1, true)
	b.p.Wait()
}

// ---------- Degradación a texto plano (stdout no es TTY) ----------

type barrasTexto struct {
	out                    io.Writer
	total, n, pasos, hecho int
}

func (b *barrasTexto) IniciarValidacion(n int, nombre string) {
	b.n, b.pasos, b.hecho = n, 0, 0
	fmt.Fprintf(b.out, "[general %d/%d] V%d %s: iniciando\n", n-1, b.total, n, nombre)
}

func (b *barrasTexto) DefinirPasos(total int) { b.pasos = total }

func (b *barrasTexto) Paso(descripcion string) {
	b.hecho++
	fmt.Fprintf(b.out, "    paso %d/%d: %s\n", b.hecho, max(b.pasos, b.hecho), descripcion)
}

func (b *barrasTexto) FinValidacion(estado string) {
	fmt.Fprintf(b.out, "[general %d/%d] V%d finalizada: %s\n", b.n, b.total, b.n, estado)
}

func (b *barrasTexto) Cerrar() {
	// no hace nada
}
