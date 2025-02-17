package reporters_search //nolint:stylecheck

import (
	"context"
	"fmt"
	"time"

	"github.com/deepfence/ThreatMapper/deepfence_server/model"
	"github.com/deepfence/ThreatMapper/deepfence_server/reporters"
	reporters_scan "github.com/deepfence/ThreatMapper/deepfence_server/reporters/scan"
	"github.com/deepfence/ThreatMapper/deepfence_utils/directory"
	"github.com/deepfence/ThreatMapper/deepfence_utils/utils"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j/dbtype"
	"github.com/rs/zerolog/log"
	"github.com/samber/mo"
)

// If no nodeIds are provided, will return all
// If no field are provided, will return all fields.
// (Fields can only be top level since neo4j does not support nested fields)
type SearchFilter struct {
	InFieldFilter []string                `json:"in_field_filter" required:"true"` // Fields to return
	Filters       reporters.FieldsFilters `json:"filters" required:"true"`
	Window        model.FetchWindow       `json:"window" required:"true"`
}

type ChainedSearchFilter struct {
	NodeFilter   SearchFilter         `json:"node_filter" required:"true"`
	RelationShip string               `json:"relation_ship" required:"true"`
	NextFilter   *ChainedSearchFilter `json:"next_filter"`
}

type SearchNodeReq struct {
	NodeFilter         SearchFilter         `json:"node_filter" required:"true"`
	ExtendedNodeFilter SearchFilter         `json:"extended_node_filter"`
	IndirectFilters    *ChainedSearchFilter `json:"related_node_filter"`
	Window             model.FetchWindow    `json:"window" required:"true"`
}

type SearchScanReq struct {
	ScanFilter          SearchFilter         `json:"scan_filters" required:"true"`
	NodeFilter          SearchFilter         `json:"node_filters" required:"true"`
	NodeIndirectFilters *ChainedSearchFilter `json:"related_node_filter"`
	Window              model.FetchWindow    `json:"window" required:"true"`
}

type SearchCountResp struct {
	Count      int              `json:"count" required:"true"`
	Categories map[string]int32 `json:"categories" required:"true"`
}

type NodeCountResp struct {
	CloudProviders    int64 `json:"cloud_provider" required:"true"`
	Host              int64 `json:"host" required:"true"`
	Container         int64 `json:"container" required:"true"`
	ContainerImage    int64 `json:"container_image" required:"true"`
	Pod               int64 `json:"pod" required:"true"`
	KubernetesCluster int64 `json:"kubernetes_cluster" required:"true"`
	Namespace         int64 `json:"namespace" required:"true"`
}

type ResultGroup struct {
	Name     string `json:"name"`
	Count    int64  `json:"count"`
	Severity string `json:"severity"`
}

type ResultGroupResp struct {
	Groups []ResultGroup `json:"groups"`
}

func CountNodes(ctx context.Context) (NodeCountResp, error) {
	res := NodeCountResp{}
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return res, err
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return res, err
	}
	defer tx.Close()

	query := `
		CALL {
			MATCH (n:Node)
			WHERE n.active = true AND n.cloud_provider <> 'internet'
			return count(n) as n1
		}
		CALL {
			MATCH (n:Container)
			WHERE n.pseudo = false AND n.active = true
			return count(n) as n2
		}
		CALL {
			MATCH (n:ContainerImage)
			WHERE n.pseudo = false AND n.active = true
			return count(n) as n3
		}
		CALL {
			MATCH (n:KubernetesCluster)
			WHERE n.active = true
			return count(n) as n4
		}
		CALL {
			MATCH (n:Pod)
			WHERE n.pseudo = false AND n.active = true
			return count(n) as n5, count(distinct n.kubernetes_namespace) as nn5
		}
		CALL {
			MATCH (n:CloudProvider)
			WHERE n.pseudo = false AND n.active = true
			return count(n) as n6
		}
		return n1, n2, n3, n4, n5, nn5, n6`
	r, err := tx.Run(query,
		map[string]interface{}{})
	if err != nil {
		return res, err
	}
	rec, err := r.Single()
	if err != nil {
		return res, err
	}

	res.Host = rec.Values[0].(int64)
	res.Container = rec.Values[1].(int64)
	res.ContainerImage = rec.Values[2].(int64)
	res.KubernetesCluster = rec.Values[3].(int64)
	res.Pod = rec.Values[4].(int64)
	res.Namespace = rec.Values[5].(int64)
	res.CloudProviders = rec.Values[6].(int64)

	return res, nil
}

func constructIndirectMatchInit(
	nodeType string,
	extendedField string,
	name string,
	filter SearchFilter,
	extendedFilter SearchFilter,
	indirectFilter *ChainedSearchFilter,
	fw model.FetchWindow,
	doReturn bool) string {
	query, prevs := constructIndirectMatch(indirectFilter, 0)

	var matchQuery string
	if nodeType == "" {
		matchQuery = `MATCH (` + name + `)`
	} else {
		matchQuery = `MATCH (` + name + `:` + nodeType + `)`
	}

	if len(prevs) == 0 {
		query += matchQuery +
			reporters.ParseFieldFilters2CypherWhereConditions(name, mo.Some(filter.Filters), true) +
			reporters.OrderFilter2CypherCondition(name, filter.Filters.OrderFilter, prevs)
	} else {
		query += matchQuery + ` -[:` + indirectFilter.RelationShip + `]- (` + prevs[len(prevs)-1] + `)` +
			reporters.ParseFieldFilters2CypherWhereConditions(name, mo.Some(filter.Filters), true) +
			reporters.OrderFilter2CypherCondition(name, filter.Filters.OrderFilter, prevs)
	}

	if doReturn {
		if extendedField != "" {
			query += "\n" + `MATCH (` + name + `) -[:IS]-> (e) ` +
				reporters.ParseFieldFilters2CypherWhereConditions("e", mo.Some(extendedFilter.Filters), true) +
				reporters.OrderFilter2CypherCondition("e", extendedFilter.Filters.OrderFilter, []string{name}) +
				` RETURN ` + reporters.FieldFilterCypher(name, filter.InFieldFilter) + `, e` +
				fw.FetchWindow2CypherQuery()
		} else {
			query += ` RETURN ` + reporters.FieldFilterCypher(name, filter.InFieldFilter) +
				fw.FetchWindow2CypherQuery()
		}
	}
	return query
}

func constructIndirectMatch(indirectFilter *ChainedSearchFilter, count int) (string, []string) {
	if indirectFilter == nil {
		return "", []string{}
	}

	name := fmt.Sprintf("n%d", count)
	query, prevs := constructIndirectMatch(indirectFilter.NextFilter, count+1)
	if len(prevs) == 0 {
		return query + `MATCH (` + name + `)` +
				reporters.ParseFieldFilters2CypherWhereConditions(name, mo.Some(indirectFilter.NodeFilter.Filters), true) +
				reporters.OrderFilter2CypherCondition(name, indirectFilter.NodeFilter.Filters.OrderFilter, prevs) + "\n",
			append(prevs, name)
	} else {
		return query + `MATCH (` + name + `)-[:` + indirectFilter.NextFilter.RelationShip + `]- (` + prevs[len(prevs)-1] + `)` +
				reporters.ParseFieldFilters2CypherWhereConditions(name, mo.Some(indirectFilter.NodeFilter.Filters), true) +
				reporters.OrderFilter2CypherCondition(name, indirectFilter.NodeFilter.Filters.OrderFilter, prevs) + "\n",
			append(prevs, name)
	}
}

func searchGenericDirectNodeReport[T reporters.Cypherable](ctx context.Context, filter SearchFilter, extendedFilter SearchFilter, indirectFilters *ChainedSearchFilter, fw model.FetchWindow) ([]T, error) {
	res := []T{}
	var dummy T

	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return res, err
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return res, err
	}
	defer tx.Close()

	query := constructIndirectMatchInit(dummy.NodeType(), dummy.ExtendedField(), "n", filter,
		extendedFilter, indirectFilters, fw, true)
	log.Debug().Msgf("search query: \n%v", query)
	r, err := tx.Run(query,
		map[string]interface{}{})

	if err != nil {
		return res, err
	}

	recs, err := r.Collect()

	if err != nil {
		return res, err
	}

	for _, rec := range recs {
		var nodeMap map[string]interface{}
		if len(filter.InFieldFilter) == 0 {
			data, has := rec.Get("n")
			if !has {
				log.Warn().Msgf("Missing neo4j entry")
				continue
			}
			da, ok := data.(dbtype.Node)
			if !ok {
				log.Warn().Msgf("Missing neo4j entry")
				continue
			}
			nodeMap = da.Props
		} else {
			nodeMap = map[string]interface{}{}
			for i := range filter.InFieldFilter {
				nodeMap[filter.InFieldFilter[i]] = rec.Values[i]
			}
		}
		isNode, _ := rec.Get("e")
		if isNode != nil {
			for k, v := range isNode.(dbtype.Node).Props {
				switch k {
				case "node_id":
					nodeMap[dummy.ExtendedField()] = v
				case "masked":
					if val, ok := nodeMap[k]; ok {
						nodeMap[k] = v.(bool) || val.(bool)
					} else {
						nodeMap[k] = v
					}
				default:
					nodeMap[k] = v
				}
			}
		}
		var node T
		utils.FromMap(nodeMap, &node)
		res = append(res, node)
	}

	return res, nil
}

func searchCloudNode(ctx context.Context, filter SearchFilter, fw model.FetchWindow) ([]model.CloudNodeAccountInfo, error) {
	res := []model.CloudNodeAccountInfo{}
	cloudProvider := filter.Filters.ContainsFilter.FieldsValues["cloud_provider"][0].(string)
	dummy := model.CloudNodeAccountInfo{
		CloudProvider: cloudProvider,
	}

	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return res, err
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return res, err
	}
	defer tx.Close()
	if cloudProvider == model.PostureProviderLinux || cloudProvider == model.PostureProviderKubernetes {
		filter.Filters.ContainsFilter.FieldsValues["agent_running"] = append(make([]interface{}, 0), true)
		delete(filter.Filters.ContainsFilter.FieldsValues, "cloud_provider")
	}
	orderFilters := filter.Filters.OrderFilter
	filter.Filters.OrderFilter = reporters.OrderFilter{}
	statusKey := "last_scan_status"
	scanStatusFilterValue, present := filter.Filters.ContainsFilter.FieldsValues[statusKey]
	scanFilter := reporters.FieldsFilters{}
	if present {
		scanFilter.ContainsFilter.FieldsValues = make(map[string][]interface{})
		scanFilter.ContainsFilter.FieldsValues[statusKey] = scanStatusFilterValue
		delete(filter.Filters.ContainsFilter.FieldsValues, statusKey)
	}

	query := `
		MATCH (n:` + dummy.NodeType() + `)` +
		reporters.ParseFieldFilters2CypherWhereConditions("n", mo.Some(filter.Filters), true) +
		` WITH n.node_id AS node_id UNWIND node_id AS x
		OPTIONAL MATCH (n:` + dummy.NodeType() + `{node_id: x})<-[:SCANNED]-(s:` + string(dummy.ScanType()) + `)-[:DETECTED]->(c:` + dummy.ScanResultType() + `)
		WITH x ` + reporters.FieldFilterCypher("", filter.InFieldFilter) + `, COUNT(c) AS total_compliance_count
		OPTIONAL MATCH (n:` + dummy.NodeType() + `{node_id: x})<-[:SCANNED]-(s:` + string(dummy.ScanType()) + `)-[:DETECTED]->(c1:` + dummy.ScanResultType() + `)
		WHERE c1.status IN $pass_status
		WITH x` + reporters.FieldFilterCypher("", filter.InFieldFilter) + `, CASE WHEN total_compliance_count = 0 THEN 0.0 ELSE COUNT(c1.status)*100.0/total_compliance_count END AS compliance_percentage
		CALL {
			WITH x
			OPTIONAL MATCH (n:` + dummy.NodeType() + `{node_id: x})<-[:SCANNED]-(s1:` + string(dummy.ScanType()) + `)
			RETURN s1.node_id AS last_scan_id, s1.status AS last_scan_status
			ORDER BY s1.updated_at DESC LIMIT 1
		}
		CALL {WITH x MATCH (n:` + dummy.NodeType() + `{node_id: x}) RETURN n.node_name as node_name, n.active as active, n.version as version}
		WITH x, node_name, version, compliance_percentage, last_scan_id, COALESCE(last_scan_status, '') as last_scan_status, active ` +
		reporters.ParseFieldFilters2CypherWhereConditions("", mo.Some(scanFilter), true) +
		`RETURN x as node_id, node_name, COALESCE(version, 'unknown') as version, compliance_percentage, COALESCE(last_scan_id, '') as last_scan_id, COALESCE(last_scan_status, '') as last_scan_status, active ` + reporters.FieldFilterCypher("", filter.InFieldFilter) +
		reporters.OrderFilter2CypherCondition("", orderFilters, nil) + fw.FetchWindow2CypherQuery()

	log.Debug().Msgf("search cloud node query: %v", query)
	r, err := tx.Run(query,
		map[string]interface{}{
			"pass_status": dummy.GetPassStatus(),
		})

	if err != nil {
		return res, err
	}

	recs, err := r.Collect()

	if err != nil {
		return res, err
	}

	for _, rec := range recs {
		var nodeMap map[string]interface{}
		if len(filter.InFieldFilter) != 0 {
			data, has := rec.Get("n")
			if !has {
				log.Warn().Msgf("Missing neo4j entry")
				continue
			}
			da, ok := data.(dbtype.Node)
			if !ok {
				log.Warn().Msgf("Missing neo4j entry")
				continue
			}
			nodeMap = da.Props
		} else {
			nodeMap = map[string]interface{}{}
			baseValuesCount := 0
			for _, nodeMapKey := range []string{"node_id", "node_name", "version", "compliance_percentage", "last_scan_id", "last_scan_status", "active"} {
				nodeMap[nodeMapKey] = rec.Values[baseValuesCount]
				baseValuesCount += 1
			}
			nodeMap["cloud_provider"] = cloudProvider
			for i := range filter.InFieldFilter {
				nodeMap[filter.InFieldFilter[i]] = rec.Values[i+baseValuesCount]
			}
		}
		var node model.CloudNodeAccountInfo
		utils.FromMap(nodeMap, &node)
		if node.CloudProvider == model.PostureProviderAWSOrg || node.CloudProvider == model.PostureProviderGCPOrg {
			node.ScanStatusMap, err = getScanStatusMap(ctx, node.NodeID, node.CloudProvider)
			if err != nil {
				log.Error().Msgf("Error in populating status of org %v", err)
			}
		}
		res = append(res, node)
	}
	return res, nil
}

func getScanStatusMap(ctx context.Context, id string, cloudProvider string) (map[string]int64, error) {
	res := map[string]int64{}
	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return res, err
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return res, err
	}
	defer tx.Close()
	query := `MATCH (n:CloudNode{cloud_provider:"` + cloudProvider + `",node_id:"` + id + `"}) -[:IS_CHILD] -> (m:CloudNode)
			WITH m.node_id AS node_id UNWIND node_id AS x
			CALL {
				WITH x
				OPTIONAL MATCH (n:CloudNode{node_id: x})<-[:SCANNED]-(s1:CloudComplianceScan)
				RETURN s1.node_id AS last_scan_id, COALESCE(s1.status, "NEVER_SCANNED") AS last_scan_status
				ORDER BY s1.updated_at DESC LIMIT 1
			}
	RETURN last_scan_status, count(last_scan_status) `
	r, err := tx.Run(query, map[string]interface{}{})
	if err != nil {
		return res, err
	}

	recs, err := r.Collect()
	if err != nil {
		return res, err
	}
	for _, rec := range recs {
		res[rec.Values[0].(string)] = rec.Values[1].(int64)
	}
	return res, nil
}

type FakeCypher struct {
	nodeType string
	extended string
}

func (fc FakeCypher) NodeType() string {
	return fc.nodeType
}

func (fc FakeCypher) ExtendedField() string {
	return fc.extended
}

func searchGenericScanInfoReport(ctx context.Context, scanType utils.Neo4jScanType,
	scanFilter SearchFilter, resourceFilter SearchFilter,
	resourceChainedFilter *ChainedSearchFilter,
	fw model.FetchWindow) ([]model.ScanInfo, error) {
	res := []model.ScanInfo{}

	driver, err := directory.Neo4jClient(ctx)
	if err != nil {
		return res, err
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return res, err
	}
	defer tx.Close()

	query := constructIndirectMatchInit("", "", "m", resourceFilter, SearchFilter{}, resourceChainedFilter, fw, false)

	query += `
		MATCH (:` + string(scanType) + `) -[:SCANNED]-> (m)` +
		reporters.ParseFieldFilters2CypherWhereConditions("m", mo.Some(resourceFilter.Filters), true) +
		`
	    WITH distinct m
		CALL {
	    WITH m
		MATCH (n:` + string(scanType) + `) -[:SCANNED]-> (m)` +
		reporters.ParseFieldFilters2CypherWhereConditions("n", mo.Some(scanFilter.Filters), true) +
		`
	    RETURN n
	    ORDER BY n.updated_at DESC` +
		scanFilter.Window.FetchWindow2CypherQuery() +
		`}` +
		` RETURN n.node_id as scan_id, n.status as status, n.status_message as status_message, n.created_at as created_at, n.updated_at as updated_at, m.node_id as node_id, COALESCE(m.node_type, m.cloud_provider) as node_type, m.node_name as node_name` +
		reporters.OrderFilter2CypherCondition("", scanFilter.Filters.OrderFilter, nil) +
		fw.FetchWindow2CypherQuery()
	log.Debug().Msgf("search query: %v", query)
	r, err := tx.Run(query,
		map[string]interface{}{})

	if err != nil {
		return res, err
	}

	recs, err := r.Collect()

	if err != nil {
		return res, err
	}

	for _, rec := range recs {

		counts, err := reporters_scan.GetSevCounts(ctx, scanType, rec.Values[0].(string))
		if err != nil {
			log.Error().Msgf("%v", err)
		}
		res = append(res, model.ScanInfo{
			ScanID:         rec.Values[0].(string),
			Status:         rec.Values[1].(string),
			StatusMessage:  rec.Values[2].(string),
			CreatedAt:      rec.Values[3].(int64),
			UpdatedAt:      rec.Values[4].(int64),
			NodeID:         rec.Values[5].(string),
			NodeType:       rec.Values[6].(string),
			NodeName:       rec.Values[7].(string),
			SeverityCounts: counts,
		})
	}

	return res, nil
}

func SearchCloudNodeReport[T reporters.Cypherable](ctx context.Context, filter SearchFilter, fw model.FetchWindow) ([]model.CloudNodeAccountInfo, error) {
	hosts, err := searchCloudNode(ctx, filter, fw)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func SearchReport[T reporters.Cypherable](ctx context.Context, filter SearchFilter, extendedFilter SearchFilter, indirectFilter *ChainedSearchFilter, fw model.FetchWindow) ([]T, error) {
	hosts, err := searchGenericDirectNodeReport[T](ctx, filter, extendedFilter, indirectFilter, fw)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func SearchScansReport(ctx context.Context, filter SearchScanReq, scanType utils.Neo4jScanType) ([]model.ScanInfo, error) {
	hosts, err := searchGenericScanInfoReport(ctx, scanType, filter.ScanFilter, filter.NodeFilter, filter.NodeIndirectFilters, filter.Window)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}
