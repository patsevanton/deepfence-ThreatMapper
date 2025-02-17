package controls

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deepfence/ThreatMapper/deepfence_utils/controls"
	"github.com/deepfence/ThreatMapper/deepfence_utils/directory"
	"github.com/deepfence/ThreatMapper/deepfence_utils/utils"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"golang.org/x/mod/semver"
)

const (
	DefaultAgentImageName = "deepfence.io"
	DefaultAgentImageTag  = "thomas"
	DefaultAgentVersion   = "0.0.1"
)

func PrepareAgentUpgradeAction(ctx context.Context, version string) (controls.Action, error) {

	url, err := GetAgentVersionTarball(ctx, version)
	if err != nil {
		return controls.Action{}, err
	}

	internalReq := controls.StartAgentUpgradeRequest{
		HomeDirectoryURL: url,
		Version:          version,
	}

	b, err := json.Marshal(internalReq)
	if err != nil {
		return controls.Action{}, err
	}

	return controls.Action{
		ID:             controls.StartAgentUpgrade,
		RequestPayload: string(b),
	}, nil
}

func ScheduleAgentUpgrade(ctx context.Context, version string, nodeIDs []string, action controls.Action) error {

	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	actionStr, err := json.Marshal(action)
	if err != nil {
		return err
	}

	_, err = tx.Run(`
		MATCH (v:AgentVersion{node_id: $version})
		MATCH (n:Node)
		WHERE n.node_id IN $node_ids
		MERGE (v) -[:SCHEDULED{status: $status, retries: 0, trigger_action: $action, updated_at: TIMESTAMP()}]-> (n)`,
		map[string]interface{}{
			"version":  version,
			"node_ids": nodeIDs,
			"status":   utils.ScanStatusStarting,
			"action":   string(actionStr),
		})

	if err != nil {
		return err
	}

	return tx.Commit()

}

func GetAgentVersionTarball(ctx context.Context, version string) (string, error) {

	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return "", err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return "", err
	}
	defer tx.Close()

	res, err := tx.Run(`
		MATCH (v:AgentVersion{node_id: $version})
		RETURN v.url`,
		map[string]interface{}{
			"version": version,
		})

	if err != nil {
		return "", err
	}

	r, err := res.Single()

	if err != nil {
		return "", err
	}

	return r.Values[0].(string), nil
}

func GetAgentPluginVersionTarball(ctx context.Context, version, pluginName string) (string, error) {

	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return "", err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return "", err
	}
	defer tx.Close()

	query := fmt.Sprintf(`
		MATCH (v:AgentVersion{node_id: $version})
		return v.url_%s`, pluginName)
	res, err := tx.Run(query,
		map[string]interface{}{
			"version": version,
		})

	if err != nil {
		return "", err
	}

	r, err := res.Single()

	if err != nil {
		return "", err
	}

	return r.Values[0].(string), nil
}

func hasPendingUpgradeOrNew(ctx context.Context, version string, nodeID string) (bool, error) {

	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return false, err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return false, err
	}
	defer tx.Close()

	res, err := tx.Run(`
		MATCH (n:Node{node_id:$node_id})
		MATCH (v:AgentVersion{node_id:$version})
		OPTIONAL MATCH (v) -[rs:SCHEDULED]-> (n)
		OPTIONAL MATCH (n) -[rv:VERSIONED]-> (v)
		RETURN rs IS NOT NULL OR rv IS NULL`,
		map[string]interface{}{
			"node_id": nodeID,
			"version": version,
		})
	if err != nil {
		return false, err
	}

	r, err := res.Single()
	if err != nil {
		// No results means new
		return true, nil
	}
	return r.Values[0].(bool), nil
}

func wasAttachedToNewer(ctx context.Context, version string, nodeID string) (bool, string, error) {
	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return false, "", err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return false, "", err
	}
	defer tx.Close()

	res, err := tx.Run(`
		MATCH (n:Node{node_id:$node_id}) -[old:VERSIONED]-> (v)
		RETURN v.node_id`,
		map[string]interface{}{
			"node_id": nodeID,
		})
	if err != nil {
		return false, "", err
	}

	rec, err := res.Single()
	if err != nil {
		return false, "", nil
	}

	prevVer := rec.Values[0].(string)

	return semver.Compare(prevVer, version) == 1, prevVer, nil
}

func CompleteAgentUpgrade(ctx context.Context, version string, nodeID string) error {

	has, err := hasPendingUpgradeOrNew(ctx, version, nodeID)

	if err != nil {
		return err
	}

	if !has {
		return nil
	}

	newer, prevVer, err := wasAttachedToNewer(ctx, version, nodeID)
	if err != nil {
		return err
	}

	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	_, err = tx.Run(`
		OPTIONAL MATCH (n:Node{node_id:$node_id}) -[old:VERSIONED]-> (v)
		DELETE old`,
		map[string]interface{}{
			"node_id": nodeID,
		})
	if err != nil {
		return err
	}

	_, err = tx.Run(`
		MERGE (n:Node{node_id:$node_id})
		MERGE (v:AgentVersion{node_id:$version})
		MERGE (n) -[r:VERSIONED]-> (v)
		WITH n, v
		OPTIONAL MATCH (v) -[r:SCHEDULED]-> (n)
		DELETE r`,
		map[string]interface{}{
			"version": version,
			"node_id": nodeID,
		})

	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	// If attached to newer, schedule an ugprade
	if newer {
		action, err := PrepareAgentUpgradeAction(ctx, prevVer)
		if err != nil {
			return err
		}
		err = ScheduleAgentUpgrade(ctx, prevVer, []string{nodeID}, action)
		if err != nil {
			return err
		}
	}

	return nil
}

func ScheduleAgentPluginEnable(ctx context.Context, version, pluginName string, nodeIDs []string, action controls.Action) error {

	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	actionStr, err := json.Marshal(action)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		MATCH (v:%sVersion{node_id: $version})
		MATCH (n:Node)
		WHERE n.node_id IN $nonternal_req := controls.EnableAgentPluginRequest{
		BinUrl:     url,
		Version:    agentUp.Version,
		PluginName: agentUp.PluginName,
		}
		de_ids
		MERGE (v) -[:SCHEDULED{status: $status, retries: 0, trigger_action: $action, updated_at: TIMESTAMP()}]-> (n)`, pluginName)

	_, err = tx.Run(query,
		map[string]interface{}{
			"version":  version,
			"node_ids": nodeIDs,
			"status":   utils.ScanStatusStarting,
			"action":   string(actionStr),
		})

	if err != nil {
		return err
	}

	return tx.Commit()

}

func ScheduleAgentPluginDisable(ctx context.Context, pluginName string, nodeIDs []string, action controls.Action) error {

	client, err := directory.Neo4jClient(ctx)
	if err != nil {
		return err
	}

	session := client.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(30 * time.Second))
	if err != nil {
		return err
	}
	defer tx.Close()

	actionStr, err := json.Marshal(action)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		MATCH (n:Node) -[:USES]-> (v:%sVersion)
		WHERE n.node_id IN $node_ids
		MERGE (v) -[:SCHEDULED{status: $status, retries: 0, trigger_action: $action, updated_at: TIMESTAMP()}]-> (n)
		SET n.status_%s = 'disabling'`, pluginName, pluginName)

	_, err = tx.Run(query,
		map[string]interface{}{
			"node_ids": nodeIDs,
			"status":   utils.ScanStatusStarting,
			"action":   string(actionStr),
		})

	if err != nil {
		return err
	}

	return tx.Commit()

}
