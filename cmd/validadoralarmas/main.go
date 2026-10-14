// Comando ValidadorAlarmas: diagnóstico de solo lectura del proceso de carga de alarmas.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"validadoralarmas/internal/config"
	"validadoralarmas/internal/logger"
	"validadoralarmas/internal/report"
	"validadoralarmas/internal/ui"
	"validadoralarmas/internal/validators"
)

// Se inyectan en compilación con -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "desconocido"
	date    = ""
	builtBy = ""
)

func main() { os.Exit(run()) }

// run devuelve: 0 = diagnóstico completado, 1 = error, 2 = uso incorrecto, 130 = interrumpido.
func run() int {

	verVersion := flag.Bool("version", false, "muestra la versión y termina")
	rutaCfg := flag.String("config", "", "ruta del JSON de configuración del validador")
	rutaLoader := flag.String("LoaderConfig", "", "ruta del JSON del proceso de carga (Loader) a validar")
	flag.Parse()
	if *verVersion {
		fmt.Printf("ValidadorAlarmas %s (commit %s, compilado %s por %s)\n", version, commit, date, builtBy)
		return 0
	}
	flag.Parse()
	if *rutaCfg == "" || *rutaLoader == "" {
		fmt.Fprintln(os.Stderr, "Uso: ValidadorAlarmas -config=<validador.json> -LoaderConfig=<loader.json>")
		return 2
	}

	cfg, err := config.CargarValidador(*rutaCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	loader, err := config.CargarLoader(*rutaLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	envFile := loader.EnvFile
	if cfg.EnvFile != "" { // el envFile del validador sobrescribe al del Loader
		envFile = cfg.EnvFile
	}
	resolver := config.NuevoResolver(envFile)

	// El AID se pide antes de instalar el manejo de señales: aún no hay recursos que liberar.
	aid, err := ui.PedirAID(os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}

	ahora := time.Now()
	masc := logger.NuevoEnmascarador()
	log, err := logger.Nuevo(cfg.LogPath, aid, cfg.LogLevel, ahora, masc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error al iniciar el log:", err)
		return 1
	}
	defer log.Cerrar()
	log.Info("inicio de ejecución", "aid", aid, "config", *rutaCfg, "loaderConfig", *rutaLoader, "logLevel", cfg.LogLevel)

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()
	go func() { // tras la primera señal se restaura el comportamiento por defecto (segundo Ctrl+C mata)
		<-ctx.Done()
		detener()
	}()

	vs := validators.Todas()
	prog := ui.NuevoProgreso(os.Stdout, len(vs))
	st := &validators.State{
		AID: aid, Cfg: cfg, Loader: loader, Res: resolver,
		Log: log, Masc: masc, Prog: prog, Ahora: ahora,
	}
	resultados := validators.Ejecutar(ctx, vs, st)
	prog.Cerrar()
	if err := st.Cerrar(); err != nil {
		log.Warn("error al cerrar conexiones", "error", err.Error())
	}

	nombres := make([]string, len(vs))
	for i, v := range vs {
		nombres[i] = v.Nombre()
	}
	datos := report.Datos{
		AID: aid, Inicio: ahora, Zona: report.NombreZona(ahora),
		Nombres: nombres, Resultados: resultados, Parcial: ctx.Err() != nil,
	}
	fmt.Print(masc.Ocultar(report.RenderTexto(datos, true)))

	ruta, err := report.Escribir(datos, cfg.ReportsPath, cfg.Report.NamePattern, cfg.Report.Format, masc)
	if err != nil {
		log.Error("no se pudo escribir el reporte", "error", err.Error())
		fmt.Fprintln(os.Stderr, "Error al escribir el reporte:", err)
		return 1
	}
	log.Info("fin de ejecución", "reporte", ruta, "parcial", datos.Parcial)
	fmt.Printf("\nReporte: %s\nLog:     %s\n", ruta, log.Ruta)
	if ctx.Err() != nil {
		return 130
	}
	return 0
}
