// Package validators implementa las validaciones V1..V7 sobre un State compartido.
package validators

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"validadoralarmas/internal/config"
	"validadoralarmas/internal/connectors"
	"validadoralarmas/internal/logger"
)

type Estado string

const (
	OK          Estado = "OK"
	NOK         Estado = "NOK"
	Advertencia Estado = "ADVERTENCIA"
	Omitida     Estado = "OMITIDA"
)

// Detalle es un par clave/valor estructurado.
type Detalle struct{ Clave, Valor string }

// Tabla de datos. SoloReporte = no se muestra en pantalla (sí en el archivo).
type Tabla struct {
	Titulo      string
	Columnas    []string
	Filas       [][]string
	SoloReporte bool
}

// Result es el resultado de una validación.
type Result struct {
	Estado   Estado
	Resumen  string
	Detalles []Detalle
	Tablas   []Tabla
	Err      error
	Duracion time.Duration
}

// Validator es una validación ejecutable.
type Validator interface {
	Nombre() string
	Run(ctx context.Context, st *State) Result
}

// Progreso lo implementa la UI.
type Progreso interface {
	IniciarValidacion(n int, nombre string)
	DefinirPasos(total int)
	Paso(descripcion string)
	FinValidacion(estado string)
}

type progresoNulo struct{}

func (progresoNulo) IniciarValidacion(int, string) {}
func (progresoNulo) DefinirPasos(int)              {}
func (progresoNulo) Paso(string)                   {}
func (progresoNulo) FinValidacion(string)          {}

// Origen es una coincidencia del AID en un archivo fuente (V3).
type Origen struct{ Gestor, Tipo, Archivo, Miembro string }

// State es el estado compartido entre validaciones.
type State struct {
	AID    string
	Cfg    *config.Validador
	Loader *config.Loader
	Res    *config.Resolver
	Log    *logger.Logger
	Masc   *logger.Enmascarador
	Prog   Progreso
	Ahora  time.Time

	DBProcess connectors.Consultor // nil si V1 no logró conectar
	DBFinal   connectors.Consultor

	Origenes        []Origen          // V3
	Volcado         []connectors.Fila // V4
	GestoresVolcado []string          // V4: gestores distintos hallados en PCA_ENM_DATA
	Tecnologias     []string          // V5: tecnologías válidas ("3G","4G","5G")

	cierres []func() error
}

func (s *State) Timeouts() connectors.Timeouts {
	return connectors.Timeouts{
		Conexion: time.Duration(s.Cfg.Timeouts.ConnectSeconds) * time.Second,
		Consulta: time.Duration(s.Cfg.Timeouts.QuerySeconds) * time.Second,
	}
}

func (s *State) DefinirPasos(n int) {
	if s.Prog != nil {
		s.Prog.DefinirPasos(n)
	}
}

func (s *State) Paso(d string) {
	s.Log.Trace("paso completado", "detalle", d)
	if s.Prog != nil {
		s.Prog.Paso(d)
	}
}

func (s *State) AgregarCierre(f func() error) { s.cierres = append(s.cierres, f) }

// Cerrar libera las conexiones abiertas.
func (s *State) Cerrar() error {
	var errs []error
	for _, f := range s.cierres {
		errs = append(errs, f())
	}
	s.cierres = nil
	return errors.Join(errs...)
}

// Todas devuelve las 7 validaciones en orden.
func Todas() []Validator {
	return []Validator{V1Conexiones{}, V2Ciclos{}, V3Archivos{}, V4Volcado{}, V5Filtros{}, V6Traspaso{}, V7Panel{}}
}

// Ejecutar corre las validaciones en secuencia; una falla nunca aborta el resto.
func Ejecutar(ctx context.Context, vs []Validator, st *State) []Result {
	if st.Prog == nil {
		st.Prog = progresoNulo{}
	}
	res := make([]Result, 0, len(vs))
	for i, v := range vs {
		n := i + 1
		id := fmt.Sprintf("V%d", n)
		if ctx.Err() != nil {
			st.Log.Warn("validación omitida por interrupción", "validacion", id)
			res = append(res, omitida("ejecución interrumpida antes de iniciar esta validación"))
			continue
		}
		st.Prog.IniciarValidacion(n, v.Nombre())
		st.Log.Info("inicio de validación", "validacion", id, "nombre", v.Nombre())
		inicio := time.Now()
		r := ejecutarSeguro(ctx, v, st)
		r.Duracion = time.Since(inicio)
		st.Log.Info("fin de validación", "validacion", id, "estado", string(r.Estado),
			"resumen", r.Resumen, "duracion", r.Duracion.String())
		if r.Err != nil {
			st.Log.Error("error en validación", "validacion", id, "error", r.Err.Error())
		}
		st.Prog.FinValidacion(string(r.Estado))
		res = append(res, r)
	}
	return res
}

func ejecutarSeguro(ctx context.Context, v Validator, st *State) (r Result) {
	defer func() {
		if p := recover(); p != nil {
			st.Log.Error("pánico recuperado en validación", "validacion", v.Nombre(), "panic", fmt.Sprint(p))
			r = Result{Estado: NOK, Resumen: "error interno inesperado en la validación", Err: fmt.Errorf("panic recuperado: %v", p)}
		}
	}()
	return v.Run(ctx, st)
}

// ---------- utilidades ----------

func omitida(motivo string) Result { return Result{Estado: Omitida, Resumen: motivo} }

func falloErr(prefijo string, err error) Result {
	return Result{Estado: NOK, Resumen: prefijo + ": " + resumenErr(err), Err: err}
}

func resumenErr(err error) string { return recortar(err.Error(), 160) }

func recortar(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}
