package connectors

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"validadoralarmas/internal/config"
	"validadoralarmas/internal/logger"
)

// OpcionesSFTP: Timeout por intento; KnownHosts vacío = no se verifica la host key (inseguro).
type OpcionesSFTP struct {
	Timeout    time.Duration
	KnownHosts string
}

// ProbarSFTP verifica conexión + handshake SSH + subsistema SFTP, con reintentos (intentos totales = retry, mínimo 1).
func ProbarSFTP(ctx context.Context, g config.GestorRes, o OpcionesSFTP, log *logger.Logger, masc *logger.Enmascarador) error {
	intentos := max(g.Reintentos, 1)
	var ultimo error
	hechos := 0
	for i := 1; i <= intentos; i++ {
		hechos = i
		log.Trace("intento SFTP", "gestor", g.ID, "intento", i, "de", intentos, "destino", g.HostPuerto())
		err := conectarUnaVez(ctx, g, o)
		if err == nil {
			log.Debug("SFTP conectado", "gestor", g.ID, "destino", g.HostPuerto(), "intento", i)
			return nil
		}
		ultimo = masc.Error(err)
		log.Debug("fallo SFTP", "gestor", g.ID, "intento", i, "error", ultimo.Error())
		if ctx.Err() != nil || i == intentos {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(i) * time.Second):
		}
	}
	return fmt.Errorf("falló tras %d intento(s): %w", hechos, ultimo)
}

func hostKeyCallback(ruta string) (ssh.HostKeyCallback, error) {
	if ruta == "" {
		return ssh.InsecureIgnoreHostKey(), nil // #nosec G106: documentado; se advierte en V1
	}
	cb, err := knownhosts.New(ruta)
	if err != nil {
		return nil, fmt.Errorf("no se pudo cargar knownHosts %q: %w", ruta, err)
	}
	return cb, nil
}

func conectarUnaVez(ctx context.Context, g config.GestorRes, o OpcionesSFTP) error {
	cb, err := hostKeyCallback(o.KnownHosts)
	if err != nil {
		return err
	}
	clave := g.Clave
	cfg := &ssh.ClientConfig{
		User: g.Usuario,
		Auth: []ssh.AuthMethod{
			ssh.Password(clave),
			ssh.KeyboardInteractive(func(_, _ string, preguntas []string, _ []bool) ([]string, error) {
				resp := make([]string, len(preguntas))
				for i := range resp {
					resp[i] = clave
				}
				return resp, nil
			}),
		},
		HostKeyCallback: cb,
		Timeout:         o.Timeout,
	}
	dialer := net.Dialer{Timeout: o.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", g.HostPuerto())
	if err != nil {
		return fmt.Errorf("conexión TCP: %w", err)
	}
	detener := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer detener()
	_ = conn.SetDeadline(time.Now().Add(o.Timeout))

	sshConn, canales, peticiones, err := ssh.NewClientConn(conn, g.HostPuerto(), cfg)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("handshake SSH: %w", err)
	}
	cliente := ssh.NewClient(sshConn, canales, peticiones)
	defer cliente.Close()
	sc, err := sftp.NewClient(cliente)
	if err != nil {
		return fmt.Errorf("subsistema SFTP: %w", err)
	}
	defer sc.Close()
	if _, err := sc.Getwd(); err != nil {
		return fmt.Errorf("sesión SFTP: %w", err)
	}
	return nil
}
