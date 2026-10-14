// Package queries contiene el SQL embebido y los planes de diagnóstico por descarte.
package queries

import (
	_ "embed"
	"strings"
)

//go:embed sql/consulta_3g.sql
var consulta3G string

//go:embed sql/consulta_4g.sql
var consulta4G string

//go:embed sql/consulta_5g.sql
var consulta5G string

// Consultas auxiliares. Los %s son identificadores validados por regex en config (nunca el AID ni el gestor).
const (
	// CiclosPorGestorTipo: %s = tabla de ciclos. Parámetros: gestor, tipo.
	CiclosPorGestorTipo = `SELECT * FROM %s WHERE gestor = ? AND tipo = ? ORDER BY id DESC LIMIT 1`
	// VolcadoPorAID: parámetro: AID.
	VolcadoPorAID = `SELECT eventPoId, GESTOR, ARCHIVO_FUENTE, nodeName, specificProblem, objectOfReference, eventTime, ceaseTime
FROM PCA_ENM_DATA WHERE eventPoId = ? LIMIT 50`
	// EvidenciaFila: parámetros: gestor, AID. Usa a.* para detectar columnas faltantes (p. ej. alarmingObject).
	EvidenciaFila = `SELECT a.* FROM PCA_ENM_DATA a WHERE a.GESTOR = ? AND a.eventPoId = ? LIMIT 1`
	// BusquedaPorAID: %s tabla, %s columna AID. Parámetro: AID.
	BusquedaPorAID = `SELECT * FROM %s WHERE %s = ? LIMIT 5`
)

const filtroAID = "\n\tAND a.eventPoId = ?"

// Condicion es una condición evaluable de una consulta de tecnología.
type Condicion struct {
	ID     string   // nombre corto
	SQL    string   // fragmento booleano (vacío para el join)
	Pasa   string   // texto cuando se cumple
	Falla  string   // texto cuando deja fuera el registro
	Campos []string // columnas de PCA_ENM_DATA que explican la falla
}

// Plan describe una tecnología: consulta base (parámetros: gestor, AID) y condiciones en orden.
type Plan struct {
	Tecnologia  string // "3G", "4G", "5G" (se usa para nombrar tablas)
	Etiqueta    string
	Consulta    string // consulta base + filtro por AID
	Join        string // cláusula JOIN con la tabla de traducción
	CondJoin    Condicion
	Condiciones []Condicion // pertenencia primero, luego exclusiones (orden de aparición)
}

// Pasos devuelve [join, pertenencia, exclusiones...].
func (p Plan) Pasos() []Condicion {
	return append([]Condicion{p.CondJoin}, p.Condiciones...)
}

// SQLPaso cuenta filas aplicando el join y las condiciones hasta el paso i (inclusive). Parámetros: gestor, AID.
func (p Plan) SQLPaso(i int) string {
	pasos := p.Pasos()
	var b strings.Builder
	b.WriteString("SELECT COUNT(*) AS n FROM PCA_ENM_DATA a ")
	b.WriteString(p.Join)
	b.WriteString(" WHERE a.GESTOR = ? AND a.eventPoId = ?")
	for _, c := range pasos[1 : i+1] {
		b.WriteString(" AND (" + c.SQL + ")")
	}
	return b.String()
}

// SQLPertenencia cuenta filas que cumplen solo la pertenencia a la tecnología (sin join). Parámetros: gestor, AID.
func (p Plan) SQLPertenencia() string {
	return "SELECT COUNT(*) AS n FROM PCA_ENM_DATA a WHERE a.GESTOR = ? AND a.eventPoId = ? AND (" + p.Condiciones[0].SQL + ")"
}

func excluyeObjeto(token string) Condicion {
	return Condicion{
		ID:     "exclusión " + token,
		SQL:    "a.objectOfReference NOT LIKE '%" + token + "%'",
		Pasa:   "objectOfReference no contiene '" + token + "'",
		Falla:  "excluida por objectOfReference LIKE '%" + token + "%'",
		Campos: []string{"objectOfReference"},
	}
}

// Planes devuelve los planes 3G, 4G y 5G.
func Planes() []Plan { return []Plan{plan3G(), plan4G(), plan5G()} }

func plan3G() Plan {
	return Plan{
		Tecnologia: "3G", Etiqueta: "3G (UMTS)",
		Consulta: strings.TrimSpace(consulta3G) + filtroAID,
		Join:     `JOIN ( SELECT DISTINCT SP, TIPO_PANEL, SEVERIDAD FROM FMA_TRANSLATE WHERE TIPO='3G') e ON ( LOWER(REPLACE(a.specificProblem,' ','')) = LOWER(REPLACE(e.SP,' ','')) )`,
		CondJoin: Condicion{
			ID: "join FMA_TRANSLATE", Pasa: "join con FMA_TRANSLATE (TIPO='3G') OK",
			Falla: "specificProblem no existe en FMA_TRANSLATE (TIPO='3G')", Campos: []string{"specificProblem"},
		},
		Condiciones: []Condicion{{
			ID: "pertenencia 3G", SQL: `a.objectOfReference LIKE '%SubNetwork=RNC%'`,
			Pasa:   "objectOfReference contiene 'SubNetwork=RNC'",
			Falla:  "objectOfReference no contiene 'SubNetwork=RNC'",
			Campos: []string{"objectOfReference"},
		}},
	}
}

func plan4G() Plan {
	return Plan{
		Tecnologia: "4G", Etiqueta: "4G (LTE)",
		Consulta: strings.TrimSpace(consulta4G) + filtroAID,
		Join:     `JOIN LTE_SPECIFIC_PROBLEM_ENM e ON (LOWER(REPLACE(e.DESCRIPCION,' ','')) = LOWER(REPLACE(a.specificProblem,' ','')) )`,
		CondJoin: Condicion{
			ID: "join LTE_SPECIFIC_PROBLEM_ENM", Pasa: "join con LTE_SPECIFIC_PROBLEM_ENM OK",
			Falla: "specificProblem no existe en LTE_SPECIFIC_PROBLEM_ENM (DESCRIPCION)", Campos: []string{"specificProblem"},
		},
		Condiciones: []Condicion{
			{
				ID: "pertenencia LTE",
				SQL: `( a.objectOfReference LIKE '%EUtranCell%'
 OR (a.objectOfReference LIKE '%SubNetwork=LTE%' AND a.objectOfReference LIKE '%AlarmPort%')
 OR (a.objectOfReference LIKE '%SubNetwork=LTE%' AND LOWER(REPLACE(a.specificProblem,' ',''))='heartbeatfailure') )`,
				Pasa:   "objectOfReference pertenece a LTE (EUtranCell, o SubNetwork=LTE con AlarmPort/HeartbeatFailure)",
				Falla:  "objectOfReference no corresponde a LTE (sin 'EUtranCell' ni SubNetwork=LTE con AlarmPort/HeartbeatFailure)",
				Campos: []string{"objectOfReference", "specificProblem"},
			},
			{
				ID:     "exclusión InconsistentConfiguration",
				SQL:    `NOT (LOWER(REPLACE(a.specificProblem,' ',''))='inconsistentconfiguration' AND a.problemText NOT LIKE '%Resource Activation Timeout%')`,
				Pasa:   "no excluida por InconsistentConfiguration sin 'Resource Activation Timeout'",
				Falla:  "excluida: specificProblem es InconsistentConfiguration y problemText no contiene 'Resource Activation Timeout'",
				Campos: []string{"specificProblem", "problemText"},
			},
			{
				ID:     "exclusión MOConfigurationExceedsResourcesEnabledByTheKey",
				SQL:    `NOT (LOWER(REPLACE(a.specificProblem,' ',''))='moconfigurationexceedsresourcesenabledbythekey' AND a.alarmingObject NOT LIKE '%EUtranCellFDD=%')`,
				Pasa:   "no excluida por MOConfigurationExceedsResourcesEnabledByTheKey sin EUtranCellFDD",
				Falla:  "excluida: specificProblem es MOConfigurationExceedsResourcesEnabledByTheKey y alarmingObject no contiene 'EUtranCellFDD='",
				Campos: []string{"specificProblem", "alarmingObject"},
			},
			excluyeObjeto("Router"), excluyeObjeto("RNC"), excluyeObjeto("NbIotCell"),
		},
	}
}

func plan5G() Plan {
	return Plan{
		Tecnologia: "5G", Etiqueta: "5G (NR)",
		Consulta: strings.TrimSpace(consulta5G) + filtroAID,
		Join:     `JOIN NR_SPECIFIC_PROBLEM_ENM e ON (LOWER(REPLACE(e.DESCRIPCION,' ','')) = LOWER(REPLACE(a.specificProblem,' ','')) )`,
		CondJoin: Condicion{
			ID: "join NR_SPECIFIC_PROBLEM_ENM", Pasa: "join con NR_SPECIFIC_PROBLEM_ENM OK",
			Falla: "specificProblem no existe en NR_SPECIFIC_PROBLEM_ENM (DESCRIPCION)", Campos: []string{"specificProblem"},
		},
		Condiciones: []Condicion{
			{
				ID:     "pertenencia NR",
				SQL:    `a.objectOfReference LIKE '%SubNetwork=NR_REgion%' OR a.objectOfReference LIKE '%NRCellDU%'`,
				Pasa:   "objectOfReference contiene 'SubNetwork=NR_REgion' o 'NRCellDU'",
				Falla:  "objectOfReference no contiene 'NRCellDU' ni 'SubNetwork=NR_REgion'",
				Campos: []string{"objectOfReference"},
			},
			excluyeObjeto("EUtranCellFDD"), excluyeObjeto("Router"), excluyeObjeto("RNC"), excluyeObjeto("NbIotCell"),
			{
				ID:     "exclusión CriticalTemperatureTakenOutOfService",
				SQL:    `NOT (LOWER(REPLACE(a.specificProblem,' ',''))='criticaltemperaturetakenoutofservice' AND a.alarmingObject NOT LIKE '%FieldReplaceableUnit=1%')`,
				Pasa:   "no excluida por CriticalTemperatureTakenOutOfService fuera de FieldReplaceableUnit=1",
				Falla:  "excluida: specificProblem es CriticalTemperatureTakenOutOfService y alarmingObject no contiene 'FieldReplaceableUnit=1'",
				Campos: []string{"specificProblem", "alarmingObject"},
			},
			{
				ID:     "exclusión HwFault",
				SQL:    `NOT (LOWER(REPLACE(a.specificProblem,' ',''))='hwfault' AND a.alarmingObject NOT LIKE '%FieldReplaceableUnit=1%')`,
				Pasa:   "no excluida por HwFault fuera de FieldReplaceableUnit=1",
				Falla:  "excluida: specificProblem es HwFault y alarmingObject no contiene 'FieldReplaceableUnit=1'",
				Campos: []string{"specificProblem", "alarmingObject"},
			},
		},
	}
}
