package validators

import (
	"context"
	"fmt"
	"strings"

	"validadoralarmas/internal/connectors"
	"validadoralarmas/internal/queries"
)

// nombreTabla reemplaza el marcador {TEC} por la tecnología (3G, 4G, 5G).
func nombreTabla(plantilla, tec string) string { return strings.ReplaceAll(plantilla, "{TEC}", tec) }

// tablaRegistro presenta un registro como tabla Campo/Valor (máx. maxCampos columnas).
func tablaRegistro(titulo string, cols []string, f connectors.Fila, maxCampos int) Tabla {
	t := Tabla{Titulo: titulo, Columnas: []string{"Campo", "Valor"}}
	for i, c := range cols {
		if i >= maxCampos {
			t.Filas = append(t.Filas, []string{"...", fmt.Sprintf("(%d columnas más)", len(cols)-maxCampos)})
			break
		}
		t.Filas = append(t.Filas, []string{c, recortar(f[c], 120)})
	}
	return t
}

// buscarAID busca el AID en una tabla de dbfinal. Tabla y columna provienen de config (validadas por regex);
// el AID siempre va como parámetro.
func buscarAID(ctx context.Context, c connectors.Consultor, aid, tabla, colAID string) (*connectors.Resultado, error) {
	return c.Consultar(ctx, fmt.Sprintf(queries.BusquedaPorAID, tabla, colAID), aid)
}
