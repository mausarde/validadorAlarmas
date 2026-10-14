// Package config carga la configuración del validador y del Loader.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FlexString acepta texto o número JSON (p. ej. "port": 22 o "port": "22").
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("se esperaba texto o número: %w", err)
	}
	*f = FlexString(n.String())
	return nil
}

// ---------- Configuración del validador (-config) ----------

type Timeouts struct {
	ConnectSeconds int `json:"connectSeconds"`
	QuerySeconds   int `json:"querySeconds"`
}

type ReporteCfg struct {
	Format      string `json:"format"`
	NamePattern string `json:"namePattern"`
}

// Esquema centraliza los nombres de tablas/columnas que son SUPOSICIONES a confirmar.
// Las tablas admiten el marcador {TEC} (3G, 4G, 5G).
type Esquema struct {
	TablaCiclos     string `json:"tablaCiclos"`
	ColFechaCiclo   string `json:"colFechaCiclo"`
	TablaAlarmas    string `json:"tablaAlarmas"`
	TablaPanel      string `json:"tablaPanel"`
	TablaHistorico  string `json:"tablaHistorico"`
	ColAIDAlarmas   string `json:"colAidAlarmas"`
	ColAIDPanel     string `json:"colAidPanel"`
	ColAIDHistorico string `json:"colAidHistorico"`
	ColMotivo       string `json:"colMotivo"`
	ColFin          string `json:"colFin"`
}

type Validador struct {
	EnvFile                  string     `json:"envFile"`
	LogPath                  string     `json:"logPath"`
	LogLevel                 string     `json:"logLevel"`
	ReportsPath              string     `json:"reportsPath"`
	Report                   ReporteCfg `json:"report"`
	MaxArchivosPorGestorTipo int        `json:"maxArchivosPorGestorTipo"`
	Timeouts                 Timeouts   `json:"timeouts"`
	KnownHosts               string     `json:"knownHosts"` // vacío = no verifica host key SFTP
	Esquema                  Esquema    `json:"esquema"`
}

var (
	reTabla = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)?$`)
	reCol   = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

func (v *Validador) aplicarDefectos() {
	def := func(p *string, d string) {
		if strings.TrimSpace(*p) == "" {
			*p = d
		}
	}
	def(&v.LogPath, "./log")
	def(&v.LogLevel, "info")
	def(&v.ReportsPath, "./reports")
	def(&v.Report.Format, "md")
	def(&v.Report.NamePattern, "{AID}_{FECHA_HORA}")
	if v.MaxArchivosPorGestorTipo == 0 {
		v.MaxArchivosPorGestorTipo = 5
	}
	if v.Timeouts.ConnectSeconds == 0 {
		v.Timeouts.ConnectSeconds = 10
	}
	if v.Timeouts.QuerySeconds == 0 {
		v.Timeouts.QuerySeconds = 30
	}

	e := &v.Esquema
	def(&e.TablaCiclos, "CONCELDAS.PCA_CICLOS")
	def(&e.ColFechaCiclo, "ciclo")
	def(&e.TablaAlarmas, "o_m.PCA_ALARMAS_{TEC}")
	def(&e.TablaPanel, "o_m.PCA_PANEL_{TEC}")
	def(&e.TablaHistorico, "o_m.PCA_HISTORICO_{TEC}")
	def(&e.ColAIDAlarmas, "AID")
	def(&e.ColAIDPanel, "AID")
	def(&e.ColAIDHistorico, "AID")
	def(&e.ColMotivo, "MOTIVO")
	def(&e.ColFin, "fin")
}

// Validar comprueba valores y que los identificadores SQL configurables sean seguros.
func (v *Validador) Validar() error {
	var errs []error
	v.Report.Format = strings.ToLower(strings.TrimSpace(v.Report.Format))
	if v.Report.Format != "md" && v.Report.Format != "txt" {
		errs = append(errs, fmt.Errorf("report.format debe ser 'md' o 'txt' (valor: %q)", v.Report.Format))
	}
	if v.MaxArchivosPorGestorTipo < 1 {
		errs = append(errs, errors.New("maxArchivosPorGestorTipo debe ser >= 1"))
	}
	if v.Timeouts.ConnectSeconds < 1 || v.Timeouts.QuerySeconds < 1 {
		errs = append(errs, errors.New("timeouts.connectSeconds y timeouts.querySeconds deben ser >= 1"))
	}
	e := v.Esquema
	for _, t := range []struct{ n, v string }{
		{"tablaCiclos", e.TablaCiclos}, {"tablaAlarmas", e.TablaAlarmas},
		{"tablaPanel", e.TablaPanel}, {"tablaHistorico", e.TablaHistorico},
	} {
		if !reTabla.MatchString(strings.ReplaceAll(t.v, "{TEC}", "3G")) {
			errs = append(errs, fmt.Errorf("esquema.%s no es un identificador válido: %q", t.n, t.v))
		}
	}
	for _, c := range []struct{ n, v string }{
		{"colFechaCiclo", e.ColFechaCiclo}, {"colAidAlarmas", e.ColAIDAlarmas},
		{"colAidPanel", e.ColAIDPanel}, {"colAidHistorico", e.ColAIDHistorico},
		{"colMotivo", e.ColMotivo}, {"colFin", e.ColFin},
	} {
		if !reCol.MatchString(c.v) {
			errs = append(errs, fmt.Errorf("esquema.%s no es un nombre de columna válido: %q", c.n, c.v))
		}
	}
	return errors.Join(errs...)
}

// CargarValidador lee y valida el JSON del validador aplicando valores por defecto.
func CargarValidador(ruta string) (*Validador, error) {
	var v Validador
	if err := leerJSON(ruta, &v); err != nil {
		return nil, err
	}
	v.aplicarDefectos()
	if err := v.Validar(); err != nil {
		return nil, fmt.Errorf("configuración del validador inválida: %w", err)
	}
	return &v, nil
}

// ---------- Configuración del Loader (-LoaderConfig) ----------

type SFTP struct {
	Server   string     `json:"server"`
	Port     FlexString `json:"port"`
	User     string     `json:"user"`
	Password string     `json:"password"`
}

type Gestor struct {
	ID                string `json:"id"`
	FilesAlarmPattern string `json:"filesAlarmPattern"`
	FilesClearPattern string `json:"filesClearPattern"`
	FilesPath         string `json:"filesPath"`
	Retry             int    `json:"retry"`
	SFTP              SFTP   `json:"sftp"`
}

type SSL struct {
	Enabled bool   `json:"enabled"`
	CACert  string `json:"cacert"`
	Cert    string `json:"cert"`
	Key     string `json:"key"`
}

type DB struct {
	Host     string     `json:"host"`
	Port     FlexString `json:"port"`
	User     string     `json:"user"`
	Password string     `json:"password"`
	Name     string     `json:"name"`
	SSL      SSL        `json:"ssl"`
}

type Almacenamiento struct {
	DBProcess DB `json:"dbprocess"`
	DBFinal   DB `json:"dbfinal"`
}

// Loader refleja el JSON del proceso de carga. Los valores "ENVFILE->clave" se resuelven
// bajo demanda con Resolver. El campo Estados se carga pero se IGNORA en esta versión.
type Loader struct {
	LogPath      string         `json:"logPath"`
	LogLevel     string         `json:"logLevel"`
	DownloadPath string         `json:"downloadPath"`
	ReadyPath    string         `json:"readyPath"`
	EnvFile      string         `json:"ENVFILE"`
	Estados      []string       `json:"estados"`
	Gestores     []Gestor       `json:"gestores"`
	Storage      Almacenamiento `json:"storage"`
}

// Validar comprueba la estructura mínima del Loader.
func (l *Loader) Validar() error {
	var errs []error
	if strings.TrimSpace(l.ReadyPath) == "" {
		errs = append(errs, errors.New("readyPath es obligatorio"))
	}
	if len(l.Gestores) == 0 {
		errs = append(errs, errors.New("debe definirse al menos un gestor"))
	}
	vistos := map[string]bool{}
	for i, g := range l.Gestores {
		if g.ID == "" {
			errs = append(errs, fmt.Errorf("gestores[%d]: id vacío", i))
			continue
		}
		if vistos[g.ID] {
			errs = append(errs, fmt.Errorf("gestor %q duplicado", g.ID))
		}
		vistos[g.ID] = true
		for nombre, patron := range map[string]string{"filesAlarmPattern": g.FilesAlarmPattern, "filesClearPattern": g.FilesClearPattern} {
			if patron == "" {
				errs = append(errs, fmt.Errorf("gestor %s: %s vacío", g.ID, nombre))
			} else if _, err := filepath.Match(patron, ""); err != nil {
				errs = append(errs, fmt.Errorf("gestor %s: %s inválido: %w", g.ID, nombre, err))
			}
		}
	}
	return errors.Join(errs...)
}

// CargarLoader lee y valida el JSON del Loader.
func CargarLoader(ruta string) (*Loader, error) {
	var l Loader
	if err := leerJSON(ruta, &l); err != nil {
		return nil, err
	}
	if err := l.Validar(); err != nil {
		return nil, fmt.Errorf("LoaderConfig inválido: %w", err)
	}
	return &l, nil
}

func leerJSON(ruta string, destino any) error {
	f, err := os.Open(ruta)
	if err != nil {
		return fmt.Errorf("no se pudo abrir %q: %w", ruta, err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(destino); err != nil {
		return fmt.Errorf("JSON inválido en %q: %w", ruta, err)
	}
	return nil
}
