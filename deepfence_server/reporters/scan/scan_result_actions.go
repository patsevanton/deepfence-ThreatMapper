package reporters_scan //nolint:stylecheck

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/deepfence/ThreatMapper/deepfence_server/model"
	"github.com/deepfence/ThreatMapper/deepfence_server/reporters"
	"github.com/deepfence/ThreatMapper/deepfence_utils/directory"
	"github.com/deepfence/ThreatMapper/deepfence_utils/log"
	"github.com/deepfence/ThreatMapper/deepfence_utils/utils"
	ingestersUtil "github.com/deepfence/ThreatMapper/deepfence_utils/utils/ingesters"
	"github.com/minio/minio-go/v7"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
)

func UpdateScanResultNodeFields(ctx context.Context, scanType utils.Neo4jScanType, scanID string, nodeIDs []string, key, value string) error {
	// (m:VulnerabilityScan) - [r:DETECTED] -> (n:Cve)
	// update fields of "Cve" node
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	if err != nil {
		return err
	}
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	_, err = tx.Run(`
		MATCH (m:`+string(scanType)+`) -[r:DETECTED]-> (n)
		WHERE n.node_id IN $node_ids AND m.node_id = $scan_id
		SET n.`+key+` = $value`, map[string]interface{}{"node_ids": nodeIDs, "value": value, "scan_id": scanID})
	if err != nil {
		return err
	}
	return tx.Commit()
}

func UpdateScanResultMasked(ctx context.Context, req *model.ScanResultsMaskRequest, value bool) error {
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	if err != nil {
		return err
	}
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	switch req.MaskAction {
	case utils.MaskGlobal:
		nodeTag := utils.ScanTypeDetectedNode[utils.Neo4jScanType(req.ScanType)]
		globalQuery := `
        MATCH (o:` + nodeTag + `) -[:IS]-> (r)
        WHERE o.node_id IN $node_ids
        MATCH (n:` + nodeTag + `) -[:IS]-> (r)
        MATCH (s) - [d:DETECTED] -> (n)
        SET r.masked = $value, n.masked = $value, d.masked = $value
        WITH s, n
        MATCH (s) -[:SCANNED] ->(e)
        MATCH (c:ContainerImage{node_id: e.docker_image_id}) -[:ALIAS] ->(t)
        MERGE (t) -[m:MASKED]->(n)
        SET m.masked = $value
		WITH c,n
		MATCH (c) -[:IS] ->(ist)
		%s`

		imageStubQuery := ""
		if value {
			imageStubQuery = `MERGE (ist) -[ma:MASKED]-> (n) SET ma.masked = true`
		} else {
			imageStubQuery = `MATCH(ist) -[ma:MASKED] -> (n) DELETE ma`
		}

		globalQuery = fmt.Sprintf(globalQuery, imageStubQuery)

		if utils.Neo4jScanType(req.ScanType) == utils.NEO4JCloudComplianceScan {
			globalQuery = `
			MATCH (o:CloudCompliance)
			WHERE o.node_id IN $node_ids
			WITH distinct(o.full_control_id) as control_ids
				MATCH (n:CloudCompliance) <-[d:DETECTED]- (s:CloudComplianceScan)
				WHERE n.full_control_id IN control_ids
				SET n.masked=$value, d.masked=$value
			WITH control_ids
				MATCH (c:CloudComplianceControl)
				WHERE c.control_id IN control_ids
				SET c.active=$active
			`
		}

		log.Debug().Msgf("mask_global query: %s", globalQuery)

		_, err = tx.Run(globalQuery, map[string]interface{}{"node_ids": req.ResultIDs, "value": value, "active": !value})

	case utils.MaskAllImageTag:
		entityQuery := `
        MATCH (s:` + string(req.ScanType) + `) - [d:DETECTED] -> (n)
        WHERE n.node_id IN $node_ids
		WITH s, n, d
		MATCH (s) -[:SCANNED]-> (c:ContainerImage) -[:ALIAS] ->(t) -[m:MASKED]-> (n)
		WITH s, n, d, m, c
		MATCH (c)-[:IS]->(ist)
		SET d.masked=$value, m.masked=$value
		WITH ist, n
		%s`

		imageStubQuery := ""
		if value {
			imageStubQuery = `MERGE (ist) -[ma:MASKED]-> (n) SET ma.masked = true`
		} else {
			imageStubQuery = `MATCH(ist) -[ma:MASKED] -> (n) DELETE ma`
		}

		entityQuery = fmt.Sprintf(entityQuery, imageStubQuery)
		log.Debug().Msgf("mask_all_image_tag query: %s", entityQuery)
		_, err = tx.Run(entityQuery, map[string]interface{}{"node_ids": req.ResultIDs,
			"value": value, "scan_id": req.ScanID})

	case utils.MaskEntity:
		entityQuery := `
        MATCH (s:` + string(req.ScanType) + `) - [d:DETECTED] -> (n)
        WHERE n.node_id IN $node_ids
        SET n.masked = $value, d.masked = $value`

		log.Debug().Msgf("mask_entity query: %s", entityQuery)

		_, err = tx.Run(entityQuery, map[string]interface{}{"node_ids": req.ResultIDs, "value": value})

	case utils.MaskImageTag:
		maskImageTagQuery := `
        MATCH (s:` + string(req.ScanType) + `) -[d:DETECTED] -> (n)
        WHERE n.node_id IN $node_ids AND s.node_id=$scan_id
        MATCH (s) -[:SCANNED] ->(e)
        MATCH (c:ContainerImage{node_id: e.docker_image_id}) -[:ALIAS] ->(t)
        MERGE (t) -[m:MASKED]->(n)
        SET m.masked = $value, d.masked = $value`

		log.Debug().Msgf("mask_image_tag query: %s", maskImageTagQuery)

		_, err = tx.Run(maskImageTagQuery,
			map[string]interface{}{"node_ids": req.ResultIDs, "value": value, "scan_id": req.ScanID})

	default:
		defaultMaskQuery := `
        MATCH (m:` + string(req.ScanType) + `) -[d:DETECTED] -> (n)
        WHERE n.node_id IN $node_ids AND m.node_id=$scan_id
        SET d.masked = $value`

		log.Debug().Msgf("mask_image_tag query: %s", defaultMaskQuery)

		_, err = tx.Run(defaultMaskQuery,
			map[string]interface{}{"node_ids": req.ResultIDs, "value": value, "scan_id": req.ScanID})

	}

	if err != nil {
		return err
	}
	return tx.Commit()
}

func DeleteScan(ctx context.Context, scanType utils.Neo4jScanType, scanID string, docIds []string) error {
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	if err != nil {
		return err
	}
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	if len(docIds) > 0 {
		_, err = tx.Run(`
		MATCH (m:`+string(scanType)+`) -[r:DETECTED]-> (n)
		WHERE n.node_id IN $node_ids AND m.node_id = $scan_id
		DELETE r`, map[string]interface{}{"node_ids": docIds, "scan_id": scanID})
		if err != nil {
			return err
		}
	} else {
		_, err = tx.Run(`
		MATCH (m:`+string(scanType)+`{node_id: $scan_id})
		OPTIONAL MATCH (m)-[r:DETECTED]-> (n:`+utils.ScanTypeDetectedNode[scanType]+`)
		DETACH DELETE m,r`, map[string]interface{}{"scan_id": scanID})
		if err != nil {
			return err
		}
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	tx2, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx2.Close()
	// Delete results which are not part of any scans now
	_, err = tx2.Run(`
		MATCH (n:`+utils.ScanTypeDetectedNode[scanType]+`)
		WHERE not (n)<-[:DETECTED]-(:`+string(scanType)+`)
		DETACH DELETE (n)`, map[string]interface{}{})
	if err != nil {
		return err
	}
	err = tx2.Commit()
	if err != nil {
		return err
	}
	if scanType == utils.NEO4JVulnerabilityScan {
		tx3, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
		if err != nil {
			return err
		}
		defer tx3.Close()
		_, err = tx3.Run(`
			MATCH (n:`+reporters.ScanResultMaskNode[scanType]+`)
			WHERE not (n)<-[:IS]-(:`+utils.ScanTypeDetectedNode[scanType]+`)
			DETACH DELETE (n)`, map[string]interface{}{})
		if err != nil {
			return err
		}
		err = tx3.Commit()
		if err != nil {
			return err
		}

		// remove sbom
		mc, err := directory.MinioClient(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to get minio client")
			return err
		}
		sbomFile := path.Join("/sbom", utils.ScanIDReplacer.Replace(scanID)+".json.gz")
		err = mc.DeleteFile(ctx, sbomFile, true, minio.RemoveObjectOptions{ForceDelete: true})
		if err != nil {
			log.Error().Err(err).Msgf("failed to delete sbom for scan id %s", scanID)
			return err
		}
		runtimeSbomFile := path.Join("/sbom", "runtime-"+utils.ScanIDReplacer.Replace(scanID)+".json")
		err = mc.DeleteFile(ctx, runtimeSbomFile, true, minio.RemoveObjectOptions{ForceDelete: true})
		if err != nil {
			log.Error().Err(err).Msgf("failed to delete runtime sbom for scan id %s", scanID)
			return err
		}
	}

	// update nodes scan result
	query := ""
	switch scanType {
	case utils.NEO4JVulnerabilityScan:
		query = `MATCH (n)
		WHERE (n:Node OR n:Container or n:ContainerImage)
		AND n.vulnerability_latest_scan_id="%s"
		SET n.vulnerability_latest_scan_id="", n.vulnerabilities_count=0, n.vulnerability_scan_status=""`
	case utils.NEO4JSecretScan:
		query = `MATCH (n)
		WHERE (n:Node OR n:Container or n:ContainerImage)
		AND n.secret_latest_scan_id="%s"
		SET n.secret_latest_scan_id="", n.secrets_count=0, n.secret_scan_status=""`
	case utils.NEO4JMalwareScan:
		query = `MATCH (n)
		WHERE (n:Node OR n:Container or n:ContainerImage)
		AND n.malware_latest_scan_id="%s"
		SET n.malware_latest_scan_id="", n.malwares_count=0, n.malware_scan_status=""`
	case utils.NEO4JComplianceScan:
		query = `MATCH (n)
		WHERE (n:Node OR n:KubernetesCluster)
		AND n.compliance_latest_scan_id="%s"
		SET n.compliance_latest_scan_id="", n.compliances_count=0, n.compliance_scan_status=""`
	case utils.NEO4JCloudComplianceScan:
		query = `MATCH (n)
		WHERE (n:CloudResource)
		AND n.cloud_compliance_latest_scan_id="%s"
		SET n.cloud_compliance_latest_scan_id="", n.cloud_compliances_count=0, n.cloud_compliance_scan_status=""`
	}

	if len(query) < 1 {
		return nil
	}

	log.Debug().Msgf("Query:%s", fmt.Sprintf(query, scanID))

	tx4, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx4.Close()
	_, err = tx4.Run(fmt.Sprintf(query, scanID), map[string]interface{}{})
	if err != nil {
		return err
	}
	err = tx4.Commit()
	if err != nil {
		return err
	}

	return nil
}

func MarkScanDeletePending(ctx context.Context, scanType utils.Neo4jScanType,
	scanIds []string) error {
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()
	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(15 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	query := `MATCH (n:%s) -[:SCANNED]-> (m)
			WHERE n.node_id IN $scan_ids
			SET n.status = $delete_pending`

	queryStr := fmt.Sprintf(query, string(scanType))

	log.Debug().Msgf("Query: %s", queryStr)

	if _, err = tx.Run(queryStr,
		map[string]interface{}{
			"scan_ids":       scanIds,
			"delete_pending": utils.ScanStatusDeletePending,
		}); err != nil {
		log.Error().Msgf("Failed to mark scans as DELETE_PENDING, Error: %s", err.Error())
		return err
	}
	return tx.Commit()
}

func StopCloudComplianceScan(ctx context.Context, scanIds []string) error {

	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(15 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	query := `MATCH (n:CloudComplianceScan{node_id: $scan_id}) -[:SCANNED]-> ()
        WHERE n.status = $in_progress
        SET n.status = $cancel_pending`

	for _, scanid := range scanIds {
		if _, err = tx.Run(query,
			map[string]interface{}{
				"scan_id":        scanid,
				"in_progress":    utils.ScanStatusInProgress,
				"cancel_pending": utils.ScanStatusCancelPending,
			}); err != nil {
			log.Error().Msgf("StopCloudComplianceScan: Error in setting the state in neo4j: %v", err)
			return err
		}
	}

	return tx.Commit()
}

func StopScan(ctx context.Context, scanType string, scanIds []string) error {
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()
	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(15 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	nodeStatusField := ingestersUtil.ScanStatusField[utils.Neo4jScanType(scanType)]

	query := `MATCH (n:%s{node_id: $scan_id}) -[:SCANNED]-> (m)
			WHERE n.status IN [$starting, $in_progress]
			WITH n,m                
			SET n.status = CASE WHEN n.status = $starting THEN $cancelled 
			ELSE $cancel_pending END, 
			m.%s = n.status`

	queryStr := fmt.Sprintf(query, scanType, nodeStatusField)
	for _, scanid := range scanIds {
		if _, err = tx.Run(queryStr,
			map[string]interface{}{
				"scan_id":        scanid,
				"starting":       utils.ScanStatusStarting,
				"in_progress":    utils.ScanStatusInProgress,
				"cancel_pending": utils.ScanStatusCancelPending,
				"cancelled":      utils.ScanStatusCancelled,
			}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func NotifyScanResult(ctx context.Context, scanType utils.Neo4jScanType, scanID string, scanIDs []string) error {
	switch scanType {
	case utils.NEO4JVulnerabilityScan:
		res, common, err := GetSelectedScanResults[model.Vulnerability](ctx, scanType, scanID, scanIDs)
		if err != nil {
			return err
		}
		if err := Notify[model.Vulnerability](ctx, res, common, string(scanType)); err != nil {
			return err
		}
	case utils.NEO4JSecretScan:
		res, common, err := GetSelectedScanResults[model.Secret](ctx, scanType, scanID, scanIDs)
		if err != nil {
			return err
		}
		if err := Notify[model.Secret](ctx, res, common, string(scanType)); err != nil {
			return err
		}
	case utils.NEO4JMalwareScan:
		res, common, err := GetSelectedScanResults[model.Malware](ctx, scanType, scanID, scanIDs)
		if err != nil {
			return err
		}
		if err := Notify[model.Malware](ctx, res, common, string(scanType)); err != nil {
			return err
		}
	case utils.NEO4JComplianceScan:
		res, common, err := GetSelectedScanResults[model.Compliance](ctx, scanType, scanID, scanIDs)
		if err != nil {
			return err
		}
		if err := Notify[model.Compliance](ctx, res, common, string(scanType)); err != nil {
			return err
		}
	case utils.NEO4JCloudComplianceScan:
		res, common, err := GetSelectedScanResults[model.CloudCompliance](ctx, scanType, scanID, scanIDs)
		if err != nil {
			return err
		}
		if err := Notify[model.CloudCompliance](ctx, res, common, string(scanType)); err != nil {
			return err
		}
	}

	return nil
}

func GetSelectedScanResults[T any](ctx context.Context, scanType utils.Neo4jScanType, scanID string, scanIDs []string) ([]T, model.ScanResultsCommon, error) {
	res := []T{}
	common := model.ScanResultsCommon{}
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return res, common, err
	}
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(15 * time.Second))
	if err != nil {
		return res, common, err
	}
	defer tx.Close()

	query := `MATCH (n:%s) -[:DETECTED]-> (m)
		WHERE m.node_id IN $scan_ids
		AND n.node_id = $scan_id
		RETURN m{.*}`

	result, err := tx.Run(fmt.Sprintf(query, scanType), map[string]interface{}{"scan_ids": scanIDs, "scan_id": scanID})
	if err != nil {
		log.Error().Msgf("NotifyScanResult: Error in getting the scan result nodes from neo4j: %v", err)
		return res, common, err
	}

	recs, err := result.Collect()
	if err != nil {
		log.Error().Msgf("NotifyScanResult: Error in collecting the scan result nodes from neo4j: %v", err)
		return res, common, err
	}

	for _, rec := range recs {
		var tmp T
		utils.FromMap(rec.Values[0].(map[string]interface{}), &tmp)
		res = append(res, tmp)
	}

	ncommonres, err := tx.Run(`
	MATCH (m:`+string(scanType)+`{node_id: $scan_id}) -[:SCANNED]-> (n)
	RETURN n{.*, scan_id: m.node_id, updated_at:m.updated_at, created_at:m.created_at}`,
		map[string]interface{}{"scan_id": scanID})
	if err != nil {
		return res, common, err
	}

	rec, err := ncommonres.Single()
	if err != nil {
		return res, common, err
	}

	utils.FromMap(rec.Values[0].(map[string]interface{}), &common)

	return res, common, err
}
