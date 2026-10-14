package validators

import (
	"context"
	"errors"
	"fmt"

	"validadoralarmas/internal/config"
	"validadoralarmas/internal/connectors"
)

// V1Conexiones prueba SFTP de cada gestor, dbprocess y dbfinal.
type V1Conexiones struct{}

func (V1Conexiones) Nombre() string { return "Conexiones" }

func (V1Conexiones) Run(ctx context.Context, st *State) Result {
	gestores := st.Loader.Gestores
	st.DefinirPasos(len(gestores) + 2)
	tabla := Tabla{Titulo: "Estado de conexiones", Columnas: []string{"Componente", "Estado", "Destino (host:puerto)", "Error"}}
	var errs []error
	nok := 0
	registrar := func(componente, destino string, err error) {
		estado, msg := "OK", ""
		if err != nil {
			estado, nok = "NOK", nok+1
			msg = recortar(st.Masc.OcultarTodo(err.Error()), 200)
			errs = append(errs, fmt.Errorf("%s: %w", componente, err))
		}
		tabla.Filas = append(tabla.Filas, []string{componente, estado, destino, msg})
		st.Paso(componente)
	}

	opts := connectors.OpcionesSFTP{Timeout: st.Timeouts().Conexion, KnownHosts: st.Cfg.KnownHosts}
	for _, g := range gestores {
		comp := "SFTP " + g.ID
		gr, err := st.Res.ResolverGestor(g)
		if err != nil {
			registrar(comp, "-", err)
			continue
		}
		st.Masc.RegistrarSecreto(gr.Clave)
		st.Masc.RegistrarIdentidad(gr.Usuario)
		registrar(comp, gr.HostPuerto(), connectors.ProbarSFTP(ctx, gr, opts, st.Log, st.Masc))
	}

	var destino string
	var err error
	st.DBProcess, destino, err = conectarBD(ctx, st, "dbprocess", st.Loader.Storage.DBProcess)
	registrar("dbprocess", destino, err)
	st.DBFinal, destino, err = conectarBD(ctx, st, "dbfinal", st.Loader.Storage.DBFinal)
	registrar("dbfinal", destino, err)

	tabla.SoloReporte = nok == 0 // en pantalla solo se muestra si hay fallas
	r := Result{Tablas: []Tabla{tabla}, Err: errors.Join(errs...)}
	if nok == 0 {
		r.Estado = OK
		r.Resumen = fmt.Sprintf("(%d gestores, dbprocess, dbfinal)", len(gestores))
	} else {
		r.Estado = NOK
		r.Resumen = fmt.Sprintf("%d de %d componentes con falla", nok, len(gestores)+2)
	}
	if st.Cfg.KnownHosts == "" && len(gestores) > 0 {
		r.Detalles = append(r.Detalles, Detalle{"Advertencia", "no se verifica la host key SFTP (configure 'knownHosts' en validador.json)"})
	}
	return r
}

// conectarBD resuelve credenciales, abre el pool y hace ping real. Si falla devuelve (nil, destino, err).
func conectarBD(ctx context.Context, st *State, nombre string, cfg config.DB) (connectors.Consultor, string, error) {
	res, err := st.Res.ResolverDB(nombre, cfg)
	if err != nil {
		return nil, "-", err
	}
	st.Masc.RegistrarSecreto(res.Clave)
	st.Masc.RegistrarIdentidad(res.Usuario)
	destino := res.HostPuerto()
	bd, err := connectors.AbrirMySQL(nombre, res, st.Timeouts(), st.Log, st.Masc)
	if err != nil {
		return nil, destino, err
	}
	pctx, cancel := context.WithTimeout(ctx, st.Timeouts().Conexion)
	defer cancel()
	if err := bd.Ping(pctx); err != nil {
		_ = bd.Cerrar()
		return nil, destino, err
	}
	st.AgregarCierre(bd.Cerrar)
	return bd, destino, nil
}
