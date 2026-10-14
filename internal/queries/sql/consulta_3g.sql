SELECT DISTINCT
	'OS3' AS NW,
	SUBSTRING_INDEX( SUBSTRING_INDEX(a.objectOfReference, 'SubNetwork=', -1) ,",", 1) AS NE,
	a.nodeName AS EQ,
	CASE
		WHEN a.presentSeverity = 'CRITICAL' THEN 'A1'
		WHEN a.presentSeverity = 'MAJOR' THEN 'A2'
		WHEN a.presentSeverity = 'MINOR' THEN 'A3'
		WHEN a.presentSeverity = 'WARNING' THEN 'O1'
		ELSE
			CASE
				WHEN a.previousSeverity = 'CRITICAL' THEN 'A1'
				WHEN a.previousSeverity = 'MAJOR' THEN 'A2'
				WHEN a.previousSeverity = 'MINOR' THEN 'A3'
				WHEN a.previousSeverity = 'WARNING' THEN 'O1'
				ELSE 'O2'
			END
	END AS clase,
	a.eventTime AS inicio,
	a.ceaseTime AS fin,
	a.eventPoId AS AID,
	'BTS' AS type,
	e.TIPO_PANEL AS subtype,
	CONCAT(e.SP, '\n',a.objectOfReference, '\n', a.problemText) AS texto,
	IF(ackTime IS NULL, "NO", "YES") as ack,
	IF(ackTime IS NULL, null, ackTime) as ack_time,
	CASE
		WHEN INSTR(a.objectOfReference, 'UtranCell=') > 0 THEN SUBSTRING_INDEX( SUBSTRING_INDEX(a.objectOfReference, 'UtranCell=', -1) ,',',1)
		ELSE 'UNSET'
	END AS CIRCUIT,
	'3G' AS tecnologia,
	CASE
		WHEN INSTR(a.objectOfReference, 'UtranCell=') > 0 THEN NULL
		ELSE a.NodeName
	END AS SITE,
	a.GESTOR AS GESTOR,
	a.ARCHIVO_FUENTE AS FUENTE
FROM PCA_ENM_DATA a
	JOIN ( SELECT DISTINCT SP, TIPO_PANEL, SEVERIDAD FROM FMA_TRANSLATE WHERE TIPO='3G') e
		ON ( LOWER(REPLACE(a.specificProblem,' ','')) = LOWER(REPLACE(e.SP,' ','')) )
WHERE a.objectOfReference LIKE '%SubNetwork=RNC%'
	AND a.GESTOR = ?
