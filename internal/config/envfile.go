package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// PrefijoEnv marca los valores que deben leerse del ENVFILE.
const PrefijoEnv = "ENVFILE->"

// Resolver resuelve valores "ENVFILE->clave". El archivo se lee una sola vez y de forma perezosa.
type Resolver struct {
	ruta  string
	once  sync.Once
	datos map[string]string
	err   error
}

func NuevoResolver(ruta string) *Resolver { return &Resolver{ruta: ruta} }

func (r *Resolver) cargar() {
	r.once.Do(func() {
		if r.ruta == "" {
			r.err = errors.New("ENVFILE no está configurado")
			return
		}
		f, err := os.Open(r.ruta)
		if err != nil {
			r.err = fmt.Errorf("no se pudo abrir ENVFILE %q: %w", r.ruta, err)
			return
		}
		defer f.Close()
		datos, err := ParsearEnv(f)
		if err != nil {
			r.err = fmt.Errorf("error leyendo ENVFILE %q: %w", r.ruta, err)
			return
		}
		r.datos = datos
	})
}

// Valor devuelve v tal cual si no tiene el prefijo; si lo tiene, devuelve el valor de la clave.
// Si la clave no existe devuelve un error que nombra la clave (nunca valores).
func (r *Resolver) Valor(v string) (string, error) {
	if !strings.HasPrefix(v, PrefijoEnv) {
		return v, nil
	}
	clave := strings.TrimSpace(strings.TrimPrefix(v, PrefijoEnv))
	r.cargar()
	if r.err != nil {
		return "", r.err
	}
	val, ok := r.datos[clave]
	if !ok {
		return "", fmt.Errorf("clave %q no encontrada en ENVFILE", clave)
	}
	return val, nil
}

// ParsearEnv lee formato shell CLAVE=VALOR: ignora comentarios, líneas vacías, 'export' y comillas.
func ParsearEnv(rd io.Reader) (map[string]string, error) {
	datos := map[string]string{}
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		linea := strings.TrimSpace(sc.Text())
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		linea = strings.TrimSpace(strings.TrimPrefix(linea, "export "))
		i := strings.IndexByte(linea, '=')
		if i <= 0 {
			continue
		}
		datos[strings.TrimSpace(linea[:i])] = limpiarValor(strings.TrimSpace(linea[i+1:]))
	}
	return datos, sc.Err()
}

func limpiarValor(v string) string {
	if v != "" && (v[0] == '"' || v[0] == '\'') {
		if j := strings.IndexByte(v[1:], v[0]); j >= 0 {
			return v[1 : 1+j]
		}
		return v[1:]
	}
	if i := strings.Index(v, " #"); i >= 0 { // comentario al final de línea
		v = strings.TrimSpace(v[:i])
	}
	return v
}

// ---------- Componentes resueltos ----------

type GestorRes struct {
	ID, PatronAlarma, PatronCese, RutaRemota string
	Reintentos                               int
	Host, Puerto, Usuario, Clave             string
}

func (g GestorRes) HostPuerto() string { return net.JoinHostPort(g.Host, g.Puerto) }

type SSLRes struct {
	Habilitado        bool
	CACert, Cert, Key string
}

type DBRes struct {
	Host, Puerto, Usuario, Clave, BD string
	SSL                              SSLRes
}

func (d DBRes) HostPuerto() string { return net.JoinHostPort(d.Host, d.Puerto) }

type acumulador struct {
	r   *Resolver
	err error
}

func (a *acumulador) v(s string) string {
	if a.err != nil {
		return ""
	}
	val, err := a.r.Valor(s)
	if err != nil {
		a.err = err
		return ""
	}
	return val
}

func validarPuerto(p string) error {
	n, err := strconv.Atoi(strings.TrimSpace(p))
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("puerto inválido %q", p)
	}
	return nil
}

// ResolverGestor resuelve todos los valores del gestor.
func (r *Resolver) ResolverGestor(g Gestor) (GestorRes, error) {
	a := &acumulador{r: r}
	res := GestorRes{
		ID: g.ID, PatronAlarma: g.FilesAlarmPattern, PatronCese: g.FilesClearPattern, Reintentos: g.Retry,
		RutaRemota: a.v(g.FilesPath),
		Host:       a.v(g.SFTP.Server), Puerto: a.v(string(g.SFTP.Port)),
		Usuario: a.v(g.SFTP.User), Clave: a.v(g.SFTP.Password),
	}
	if a.err != nil {
		return GestorRes{}, fmt.Errorf("gestor %s: %w", g.ID, a.err)
	}
	if res.Host == "" {
		return GestorRes{}, fmt.Errorf("gestor %s: servidor SFTP vacío", g.ID)
	}
	if err := validarPuerto(res.Puerto); err != nil {
		return GestorRes{}, fmt.Errorf("gestor %s: %w", g.ID, err)
	}
	return res, nil
}

// ResolverDB resuelve los valores de una base de datos.
func (r *Resolver) ResolverDB(nombre string, d DB) (DBRes, error) {
	a := &acumulador{r: r}
	res := DBRes{
		Host: a.v(d.Host), Puerto: a.v(string(d.Port)), Usuario: a.v(d.User),
		Clave: a.v(d.Password), BD: a.v(d.Name),
		SSL: SSLRes{Habilitado: d.SSL.Enabled, CACert: a.v(d.SSL.CACert), Cert: a.v(d.SSL.Cert), Key: a.v(d.SSL.Key)},
	}
	if a.err != nil {
		return DBRes{}, fmt.Errorf("%s: %w", nombre, a.err)
	}
	if res.Host == "" {
		return DBRes{}, fmt.Errorf("%s: host vacío", nombre)
	}
	if err := validarPuerto(res.Puerto); err != nil {
		return DBRes{}, fmt.Errorf("%s: %w", nombre, err)
	}
	return res, nil
}
