// Package connectors implementa los accesos a MySQL/MariaDB (solo lectura) y SFTP.
package connectors

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"

	"validadoralarmas/internal/config"
	"validadoralarmas/internal/logger"
)

const maxFilas = 1000

// Timeouts agrupa los tiempos máximos de conexión y consulta.
type Timeouts struct{ Conexion, Consulta time.Duration }

// Fila: columna -> valor como texto. Los NULL se representan como "NULL".
type Fila map[string]string

// Obtener busca una columna sin distinguir mayúsculas.
func (f Fila) Obtener(nombre string) (string, bool) {
	if v, ok := f[nombre]; ok {
		return v, true
	}
	for k, v := range f {
		if strings.EqualFold(k, nombre) {
			return v, true
		}
	}
	return "", false
}

// Resultado de una consulta (columnas en orden + filas).
type Resultado struct {
	Columnas []string
	Filas    []Fila
}

// Consultor abstrae la ejecución de consultas de solo lectura (facilita pruebas).
type Consultor interface {
	Consultar(ctx context.Context, consulta string, args ...any) (*Resultado, error)
}

// BD es un pool de conexiones MySQL/MariaDB de solo lectura.
type BD struct {
	nombre, destino string
	db              *sql.DB
	t               Timeouts
	log             *logger.Logger
	masc            *logger.Enmascarador
	txSoloLectura   atomic.Bool // true si el servidor admite START TRANSACTION READ ONLY
	version         atomic.Value
}

// AbrirMySQL crea el pool (no conecta hasta el primer uso/ping).
func AbrirMySQL(nombre string, c config.DBRes, t Timeouts, log *logger.Logger, masc *logger.Enmascarador) (*BD, error) {
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = c.Usuario, c.Clave, "tcp", c.HostPuerto(), c.BD
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = t.Conexion, t.Consulta, t.Consulta
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	if c.SSL.Habilitado {
		tc, err := configTLS(c)
		if err != nil {
			return nil, masc.Error(fmt.Errorf("configuración TLS de %s: %w", nombre, err))
		}
		cfg.TLS = tc
	}
	conector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, masc.Error(fmt.Errorf("configuración de %s: %w", nombre, err))
	}
	db := sql.OpenDB(conector)
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &BD{nombre: nombre, destino: c.HostPuerto(), db: db, t: t, log: log, masc: masc}, nil
}

func configTLS(c config.DBRes) (*tls.Config, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.Host}
	if c.SSL.CACert != "" {
		pem, err := os.ReadFile(c.SSL.CACert)
		if err != nil {
			return nil, fmt.Errorf("leyendo cacert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("cacert no contiene certificados PEM válidos")
		}
		tc.RootCAs = pool
	}
	if c.SSL.Cert != "" || c.SSL.Key != "" {
		par, err := tls.LoadX509KeyPair(c.SSL.Cert, c.SSL.Key)
		if err != nil {
			return nil, fmt.Errorf("cargando cert/key de cliente: %w", err)
		}
		tc.Certificates = []tls.Certificate{par}
	}
	return tc, nil
}

func (b *BD) falla(accion string, err error) error {
	return b.masc.Error(fmt.Errorf("%s en %s (%s): %w", accion, b.nombre, b.destino, err))
}

// Ping hace una conexión real y detecta la versión del servidor para decidir si usa
// transacciones de solo lectura (no disponibles en MySQL < 5.6.5 ni MariaDB < 10.0).
func (b *BD) Ping(ctx context.Context) error {
	b.log.Debug("ping a base de datos", "bd", b.nombre, "destino", b.destino)
	if err := b.db.PingContext(ctx); err != nil {
		return b.falla("ping", err)
	}
	var v string
	if err := b.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err != nil {
		// Sin versión conocida se asume que no hay transacción RO (el usuario es de solo SELECT).
		b.log.Warn("no se pudo obtener la versión del servidor", "bd", b.nombre, "error", b.masc.Error(err).Error())
		return nil
	}
	b.version.Store(v)
	ro := SoportaTxSoloLectura(v)
	b.txSoloLectura.Store(ro)
	b.log.Info("versión de base de datos detectada", "bd", b.nombre, "version", v, "txSoloLectura", ro)
	return nil
}

var reVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

// SoportaTxSoloLectura indica si la versión admite START TRANSACTION READ ONLY:
// MySQL >= 5.6.5 o MariaDB >= 10.0.
func SoportaTxSoloLectura(version string) bool {
	maria := strings.Contains(strings.ToLower(version), "mariadb")
	// Algunos servidores MariaDB anteponen "5.5.5-" por compatibilidad de replicación.
	version = strings.TrimPrefix(version, "5.5.5-")
	m := reVersion.FindStringSubmatch(version)
	if m == nil {
		return false
	}
	mayor, _ := strconv.Atoi(m[1])
	menor, _ := strconv.Atoi(m[2])
	parche, _ := strconv.Atoi(m[3])
	if maria {
		return mayor >= 10
	}
	return mayor > 5 || (mayor == 5 && (menor > 6 || (menor == 6 && parche >= 5)))
}

// esSelect garantiza que solo se ejecuten consultas de lectura.
func esSelect(consulta string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(consulta)), "SELECT")
}

type consultaCtx interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Consultar ejecuta una consulta SELECT parametrizada. Usa transacción de solo lectura si el
// servidor la soporta; si no, ejecuta la consulta directa.
func (b *BD) Consultar(ctx context.Context, consulta string, args ...any) (*Resultado, error) {
	if !esSelect(consulta) {
		return nil, b.falla("validar consulta", errors.New("solo se permiten consultas SELECT"))
	}
	ctx, cancel := context.WithTimeout(ctx, b.t.Consulta)
	defer cancel()
	inicio := time.Now()
	b.log.Debug("ejecutando consulta", "bd", b.nombre, "sql", strings.Join(strings.Fields(consulta), " "), "params", fmt.Sprintf("%v", args))

	var q consultaCtx = b.db
	if b.txSoloLectura.Load() {
		tx, err := b.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, b.falla("iniciar transacción de solo lectura", err)
		}
		defer func() { _ = tx.Rollback() }()
		q = tx
	}
	rows, err := q.QueryContext(ctx, consulta, args...)
	if err != nil {
		return nil, b.falla("ejecutar consulta", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, b.falla("leer columnas", err)
	}
	res := &Resultado{Columnas: cols}
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, b.falla("leer fila", err)
		}
		f := make(Fila, len(cols))
		for i, c := range cols {
			if vals[i].Valid {
				f[c] = vals[i].String
			} else {
				f[c] = "NULL"
			}
		}
		res.Filas = append(res.Filas, f)
		if len(res.Filas) >= maxFilas {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, b.falla("recorrer resultados", err)
	}
	b.log.Debug("consulta finalizada", "bd", b.nombre, "filas", len(res.Filas), "duracion", time.Since(inicio).String())
	return res, nil
}

// Cerrar libera el pool.
func (b *BD) Cerrar() error { return b.db.Close() }
