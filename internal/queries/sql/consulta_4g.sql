SELECT
	'OS3' AS NW,
	SUBSTRING_INDEX( SUBSTRING_INDEX(a.objectOfReference, 'SubNetwork=', -1) ,",", 1) AS NE,
	a.nodeName AS EQ,
	e.SEVERIDAD AS clase,
	a.eventTime AS inicio,
	a.ceaseTime AS fin,
	a.eventPoId AS AID,
	'eNodeB' AS type,
	IF( INSTR(objectOfReference, 'EUtranCellTDD=') = 0 AND INSTR(objectOfReference, 'EUtranCellFDD=') = 0,
		CASE
			WHEN LOWER(a.problemText) LIKE '%corte de energia ac%' OR LOWER(a.problemText) LIKE '%corte general ac%' THEN 'ENERGIA'
			WHEN LOWER(a.problemText) LIKE '%falla rectificador%' OR LOWER(a.problemText) LIKE '%falla de rectificador%' THEN 'ENERGIA'
			WHEN LOWER(a.problemText) LIKE '%temperatura%' THEN 'TEMPERATURA'
			WHEN LOWER(a.problemText) LIKE '%puerta abierta%' THEN 'PUERTA'
			WHEN LOWER(a.problemText) LIKE '%falla de gg%' THEN 'ENERGIA'
			WHEN LOWER(a.problemText) LIKE '%gg en vacío%' THEN 'ENERGIA'
			WHEN LOWER(a.problemText) LIKE '%gg con carga%' THEN 'ENERGIA'
			WHEN LOWER(a.problemText) LIKE '%bajo nivel de combustible%' THEN 'ENERGIA'
			ELSE e.TIPO_PANEL
		END,
		e.TIPO_PANEL
	) AS subtype,
	CONCAT(e.DESCRIPCION,'\n', a.objectOfReference, '\n', a.problemText) AS texto,
	IF(a.ackTime IS NULL, "NO", "YES") as ack,
	IF(a.ackTime IS NULL, null, a.ackTime) as ack_time,
	CASE
		WHEN INSTR(objectOfReference, 'EUtranCellTDD=') > 0 THEN SUBSTRING_INDEX( SUBSTRING_INDEX(a.objectOfReference, 'EUtranCellTDD=', -1) ,",", 1)
		WHEN INSTR(objectOfReference, 'EUtranCellFDD=') > 0 THEN SUBSTRING_INDEX( SUBSTRING_INDEX(a.objectOfReference, 'EUtranCellFDD=', -1) ,",", 1)
		ELSE 'UNSET'
	END AS CIRCUIT,
	'LTE' AS tecnologia,
	CASE
		WHEN a.NodeName="" AND INSTR(a.objectOfReference, 'MeContext=') > 0 THEN SUBSTRING_INDEX( SUBSTRING_INDEX(a.objectOfReference, 'MeContext=', -1) ,",", 1)
		WHEN a.NodeName="" AND INSTR(a.objectOfReference, 'ManagedElement=') > 0 THEN SUBSTRING_INDEX( SUBSTRING_INDEX(a.objectOfReference, 'ManagedElement=', -1) ,",", 1)
		ELSE a.NodeName
	END AS SITE,
	a.GESTOR AS GESTOR,
	a.ARCHIVO_FUENTE AS FUENTE
FROM PCA_ENM_DATA a
	JOIN LTE_SPECIFIC_PROBLEM_ENM e ON (LOWER(REPLACE(e.DESCRIPCION,' ',''))  = LOWER(REPLACE(a.specificProblem,' ','')) )
WHERE
	( a.objectOfReference LIKE '%EUtranCell%'
	  OR (objectOfReference LIKE '%SubNetwork=LTE%' AND objectOfReference LIKE '%AlarmPort%' )
	  OR (objectOfReference LIKE '%SubNetwork=LTE%' AND LOWER(REPLACE(a.specificProblem,' ',''))='heartbeatfailure' )
	)
	AND NOT (LOWER(REPLACE(a.specificProblem,' ',''))='inconsistentconfiguration' AND a.problemText NOT LIKE '%Resource Activation Timeout%' )
	AND NOT (LOWER(REPLACE(a.specificProblem,' ',''))='moconfigurationexceedsresourcesenabledbythekey' AND a.alarmingObject NOT LIKE '%EUtranCellFDD=%' )
	AND a.objectOfReference NOT LIKE '%Router%'
	AND a.objectOfReference NOT LIKE '%RNC%'
	AND a.objectOfReference NOT LIKE '%NbIotCell%'
	AND a.GESTOR = ?
