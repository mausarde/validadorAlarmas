// Package logger ofrece logging por niveles (incluye trace) y enmascarado de secretos.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// NivelTrace es más fino que debug.
const NivelTrace = slog.Level(-8)

const marca = "***"

// Enmascarador reemplaza secretos registrados. Los "secretos" (contraseñas) se ocultan en todo
// texto de salida; las "identidades" (usuarios) solo se ocultan en mensajes de error (OcultarTodo).
// Se ignoran valores de menos de 3 caracteres para no destruir el texto.
type Enmascarador struct {
	mu          sync.RWMutex
	secretos    []string
	identidades []string
}

func NuevoEnmascarador() *Enmascarador { return &Enmascarador{} }

func insertar(lista []string, s string) []string {
	if len(s) < 3 {
		return lista
	}
	for _, x := range lista {
		if x == s {
			return lista
		}
	}
	lista = append(lista, s)
	sort.Slice(lista, func(i, j int) bool { return len(lista[i]) > len(lista[j]) })
	return lista
}

func (m *Enmascarador) RegistrarSecreto(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secretos = insertar(m.secretos, s)
}

func (m *Enmascarador) RegistrarIdentidad(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.identidades = insertar(m.identidades, s)
}

func reemplazar(texto string, lista []string) string {
	for _, s := range lista {
		texto = strings.ReplaceAll(texto, s, marca)
	}
	return texto
}

// Ocultar enmascara los secretos (contraseñas).
func (m *Enmascarador) Ocultar(texto string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return reemplazar(texto, m.secretos)
}

// OcultarTodo enmascara secretos e identidades.
func (m *Enmascarador) OcultarTodo(texto string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return reemplazar(reemplazar(texto, m.secretos), m.identidades)
}

type errLimpio struct {
	msg   string
	causa error
}

func (e *errLimpio) Error() string { return e.msg }
func (e *errLimpio) Unwrap() error { return e.causa }

// Error devuelve un error cuyo mensaje no contiene secretos ni identidades (conserva la cadena de errores).
func (m *Enmascarador) Error(err error) error {
	if err == nil {
		return nil
	}
	return &errLimpio{msg: m.OcultarTodo(err.Error()), causa: err}
}

type escritorMascara struct {
	w io.Writer
	m *Enmascarador
}

func (e escritorMascara) Write(p []byte) (int, error) {
	if _, err := e.w.Write([]byte(e.m.Ocultar(string(p)))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Logger envuelve slog.Logger agregando el nivel trace.
type Logger struct {
	*slog.Logger
	archivo *os.File
	Ruta    string
}

func (l *Logger) Trace(msg string, args ...any) {
	l.Logger.Log(context.Background(), NivelTrace, msg, args...)
}

// Cerrar cierra el archivo de log.
func (l *Logger) Cerrar() error {
	if l.archivo == nil {
		return nil
	}
	return l.archivo.Close()
}

func ParseNivel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return NivelTrace, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("logLevel %q inválido (use trace, debug, info, warn o error)", s)
}

// NombreArchivo devuelve ValidadorAlarmas_[AID]_[FECHA_HORA].log.
func NombreArchivo(aid string, t time.Time) string {
	return fmt.Sprintf("ValidadorAlarmas_%s_%s.log", aid, t.Format("20060102_150405"))
}

// Nuevo crea un logger a archivo (uno por ejecución) con enmascarado de secretos.
func Nuevo(dir, aid, nivel string, ahora time.Time, m *Enmascarador) (*Logger, error) {
	lv, err := ParseNivel(nivel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("no se pudo crear logPath %q: %w", dir, err)
	}
	ruta := filepath.Join(dir, NombreArchivo(aid, ahora))
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el log %q: %w", ruta, err)
	}
	h := slog.NewTextHandler(escritorMascara{w: f, m: m}, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if l, ok := a.Value.Any().(slog.Level); ok && l == NivelTrace {
					a.Value = slog.StringValue("TRACE")
				}
			}
			return a
		},
	})
	return &Logger{Logger: slog.New(h), archivo: f, Ruta: ruta}, nil
}

// Descartar devuelve un logger que no escribe nada (útil en pruebas).
func Descartar() *Logger {
	return &Logger{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}
