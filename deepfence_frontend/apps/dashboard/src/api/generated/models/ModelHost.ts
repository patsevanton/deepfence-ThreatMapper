/* tslint:disable */
/* eslint-disable */
/**
 * Deepfence ThreatMapper
 * Deepfence Runtime API provides programmatic control over Deepfence microservice securing your container, kubernetes and cloud deployments. The API abstracts away underlying infrastructure details like cloud provider,  container distros, container orchestrator and type of deployment. This is one uniform API to manage and control security alerts, policies and response to alerts for microservices running anywhere i.e. managed pure greenfield container deployments or a mix of containers, VMs and serverless paradigms like AWS Fargate.
 *
 * The version of the OpenAPI document: 2.0.0
 * Contact: community@deepfence.io
 *
 * NOTE: This class is auto generated by OpenAPI Generator (https://openapi-generator.tech).
 * https://openapi-generator.tech
 * Do not edit the class manually.
 */

import { exists, mapValues } from '../runtime';
import type { ModelComputeMetrics } from './ModelComputeMetrics';
import {
    ModelComputeMetricsFromJSON,
    ModelComputeMetricsFromJSONTyped,
    ModelComputeMetricsToJSON,
} from './ModelComputeMetrics';
import type { ModelContainer } from './ModelContainer';
import {
    ModelContainerFromJSON,
    ModelContainerFromJSONTyped,
    ModelContainerToJSON,
} from './ModelContainer';
import type { ModelContainerImage } from './ModelContainerImage';
import {
    ModelContainerImageFromJSON,
    ModelContainerImageFromJSONTyped,
    ModelContainerImageToJSON,
} from './ModelContainerImage';
import type { ModelPod } from './ModelPod';
import {
    ModelPodFromJSON,
    ModelPodFromJSONTyped,
    ModelPodToJSON,
} from './ModelPod';
import type { ModelProcess } from './ModelProcess';
import {
    ModelProcessFromJSON,
    ModelProcessFromJSONTyped,
    ModelProcessToJSON,
} from './ModelProcess';

/**
 * 
 * @export
 * @interface ModelHost
 */
export interface ModelHost {
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    cloud_compliance_scan_status: string;
    /**
     * 
     * @type {number}
     * @memberof ModelHost
     */
    cloud_compliances_count: number;
    /**
     * 
     * @type {{ [key: string]: any; }}
     * @memberof ModelHost
     */
    cloud_metadata: { [key: string]: any; };
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    compliance_scan_status: string;
    /**
     * 
     * @type {number}
     * @memberof ModelHost
     */
    compliances_count: number;
    /**
     * 
     * @type {Array<ModelContainerImage>}
     * @memberof ModelHost
     */
    container_images: Array<ModelContainerImage> | null;
    /**
     * 
     * @type {Array<ModelContainer>}
     * @memberof ModelHost
     */
    containers: Array<ModelContainer> | null;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    host_name: string;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    interfaceNames: string;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    interface_ips: string;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    kernel_version: string;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    malware_scan_status: string;
    /**
     * 
     * @type {number}
     * @memberof ModelHost
     */
    malwares_count: number;
    /**
     * 
     * @type {ModelComputeMetrics}
     * @memberof ModelHost
     */
    metrics: ModelComputeMetrics;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    node_id: string;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    node_name: string;
    /**
     * 
     * @type {Array<ModelPod>}
     * @memberof ModelHost
     */
    pods: Array<ModelPod> | null;
    /**
     * 
     * @type {Array<ModelProcess>}
     * @memberof ModelHost
     */
    processes: Array<ModelProcess> | null;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    secret_scan_status: string;
    /**
     * 
     * @type {number}
     * @memberof ModelHost
     */
    secrets_count: number;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    uptime: string;
    /**
     * 
     * @type {number}
     * @memberof ModelHost
     */
    vulnerabilities_count: number;
    /**
     * 
     * @type {string}
     * @memberof ModelHost
     */
    vulnerability_scan_status: string;
}

/**
 * Check if a given object implements the ModelHost interface.
 */
export function instanceOfModelHost(value: object): boolean {
    let isInstance = true;
    isInstance = isInstance && "cloud_compliance_scan_status" in value;
    isInstance = isInstance && "cloud_compliances_count" in value;
    isInstance = isInstance && "cloud_metadata" in value;
    isInstance = isInstance && "compliance_scan_status" in value;
    isInstance = isInstance && "compliances_count" in value;
    isInstance = isInstance && "container_images" in value;
    isInstance = isInstance && "containers" in value;
    isInstance = isInstance && "host_name" in value;
    isInstance = isInstance && "interfaceNames" in value;
    isInstance = isInstance && "interface_ips" in value;
    isInstance = isInstance && "kernel_version" in value;
    isInstance = isInstance && "malware_scan_status" in value;
    isInstance = isInstance && "malwares_count" in value;
    isInstance = isInstance && "metrics" in value;
    isInstance = isInstance && "node_id" in value;
    isInstance = isInstance && "node_name" in value;
    isInstance = isInstance && "pods" in value;
    isInstance = isInstance && "processes" in value;
    isInstance = isInstance && "secret_scan_status" in value;
    isInstance = isInstance && "secrets_count" in value;
    isInstance = isInstance && "uptime" in value;
    isInstance = isInstance && "vulnerabilities_count" in value;
    isInstance = isInstance && "vulnerability_scan_status" in value;

    return isInstance;
}

export function ModelHostFromJSON(json: any): ModelHost {
    return ModelHostFromJSONTyped(json, false);
}

export function ModelHostFromJSONTyped(json: any, ignoreDiscriminator: boolean): ModelHost {
    if ((json === undefined) || (json === null)) {
        return json;
    }
    return {
        
        'cloud_compliance_scan_status': json['cloud_compliance_scan_status'],
        'cloud_compliances_count': json['cloud_compliances_count'],
        'cloud_metadata': json['cloud_metadata'],
        'compliance_scan_status': json['compliance_scan_status'],
        'compliances_count': json['compliances_count'],
        'container_images': (json['container_images'] === null ? null : (json['container_images'] as Array<any>).map(ModelContainerImageFromJSON)),
        'containers': (json['containers'] === null ? null : (json['containers'] as Array<any>).map(ModelContainerFromJSON)),
        'host_name': json['host_name'],
        'interfaceNames': json['interfaceNames'],
        'interface_ips': json['interface_ips'],
        'kernel_version': json['kernel_version'],
        'malware_scan_status': json['malware_scan_status'],
        'malwares_count': json['malwares_count'],
        'metrics': ModelComputeMetricsFromJSON(json['metrics']),
        'node_id': json['node_id'],
        'node_name': json['node_name'],
        'pods': (json['pods'] === null ? null : (json['pods'] as Array<any>).map(ModelPodFromJSON)),
        'processes': (json['processes'] === null ? null : (json['processes'] as Array<any>).map(ModelProcessFromJSON)),
        'secret_scan_status': json['secret_scan_status'],
        'secrets_count': json['secrets_count'],
        'uptime': json['uptime'],
        'vulnerabilities_count': json['vulnerabilities_count'],
        'vulnerability_scan_status': json['vulnerability_scan_status'],
    };
}

export function ModelHostToJSON(value?: ModelHost | null): any {
    if (value === undefined) {
        return undefined;
    }
    if (value === null) {
        return null;
    }
    return {
        
        'cloud_compliance_scan_status': value.cloud_compliance_scan_status,
        'cloud_compliances_count': value.cloud_compliances_count,
        'cloud_metadata': value.cloud_metadata,
        'compliance_scan_status': value.compliance_scan_status,
        'compliances_count': value.compliances_count,
        'container_images': (value.container_images === null ? null : (value.container_images as Array<any>).map(ModelContainerImageToJSON)),
        'containers': (value.containers === null ? null : (value.containers as Array<any>).map(ModelContainerToJSON)),
        'host_name': value.host_name,
        'interfaceNames': value.interfaceNames,
        'interface_ips': value.interface_ips,
        'kernel_version': value.kernel_version,
        'malware_scan_status': value.malware_scan_status,
        'malwares_count': value.malwares_count,
        'metrics': ModelComputeMetricsToJSON(value.metrics),
        'node_id': value.node_id,
        'node_name': value.node_name,
        'pods': (value.pods === null ? null : (value.pods as Array<any>).map(ModelPodToJSON)),
        'processes': (value.processes === null ? null : (value.processes as Array<any>).map(ModelProcessToJSON)),
        'secret_scan_status': value.secret_scan_status,
        'secrets_count': value.secrets_count,
        'uptime': value.uptime,
        'vulnerabilities_count': value.vulnerabilities_count,
        'vulnerability_scan_status': value.vulnerability_scan_status,
    };
}

