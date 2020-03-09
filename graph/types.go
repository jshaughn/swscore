// Graph package provides support for the graph handlers such as supported path
// variables and query params, as well as types for graph processing.
package graph

import (
	"fmt"
	"time"
)

const (
	GraphTypeApp          string = "app"
	GraphTypeService      string = "service" // Treated as graphType Workload, with service injection, and then condensed
	GraphTypeVersionedApp string = "versionedApp"
	GraphTypeWorkload     string = "workload"
	NodeTypeApp           string = "app"
	NodeTypeService       string = "service"
	NodeTypeUnknown       string = "unknown" // The special "unknown" traffic gen node
	NodeTypeWorkload      string = "workload"
	TF                    string = "2006-01-02 15:04:05" // TF is the TimeFormat for timestamps
	Unknown               string = "unknown"             // Istio unknown label value
	// private
	passthroughCluster string = "PassthroughCluster"
	blackHoleCluster   string = "BlackHoleCluster"
)

type Node struct {
	ID        string   // unique identifier for the node
	NodeType  string   // Node type
	Namespace string   // Namespace
	Workload  string   // Workload (deployment) name
	App       string   // Workload app label value
	Version   string   // Workload version label value
	Service   string   // Service name
	Edges     []*Edge  // child nodes
	Metadata  Metadata // app-specific data
}

type Edge struct {
	Source   *Node
	Dest     *Node
	Metadata Metadata // app-specific data
}

type NamespaceInfo struct {
	Name     string
	Duration time.Duration
	IsIstio  bool
}

type NamespaceInfoMap map[string]NamespaceInfo

func NewNamespaceInfoMap() NamespaceInfoMap {
	return make(map[string]NamespaceInfo)
}

// GetIstioNamespaces returns all Istio namespaces found in the NamespaceInfo value set
func (in NamespaceInfoMap) GetIstioNamespaces() []string {
	result := []string{}
	for _, info := range in {
		if info.IsIstio {
			result = append(result, info.Name)
		}
	}
	return result
}

type ServiceName struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func (s *ServiceName) Key() string {
	return fmt.Sprintf("%s %s", s.Namespace, s.Name)
}

// TrafficMap is a map of app Nodes, each optionally holding Edge data. Metadata
// is a general purpose map for holding any desired node or edge information.
// Each app node should have a unique namespace+workload.  Note that it is feasible
// but likely unusual to have two nodes with the same name+version in the same
// namespace.
type TrafficMap map[string]*Node

// NewNode allocates a new Node object for the given telemetry
func NewNode(serviceNamespace, serviceName string, workloadNamespace, workload, app, version, graphType string) Node {
	id, nodeType := NodeID(serviceNamespace, serviceName, workloadNamespace, workload, app, version, graphType)
	namespace := workloadNamespace
	if !IsOK(namespace) {
		namespace = serviceNamespace
	}

	return NewNodeExplicit(id, namespace, workload, app, version, serviceName, nodeType, graphType)
}

// NewNodeExplicit allocates a new Node object using the provided ID and nodeType
func NewNodeExplicit(id, namespace, workload, app, version, serviceName, nodeType, graphType string) Node {
	metadata := make(Metadata)

	// trim unnecessary fields
	switch nodeType {
	case NodeTypeWorkload:
		// maintain the app+version labeling if it is set, it can be useful
		// for identifying destination rules, providing links, and grouping
		if app == Unknown {
			app = ""
		}
		if version == Unknown {
			version = ""
		}
		serviceName = ""
	case NodeTypeApp:
		// note: we keep workload for a versioned app node because app+version labeling
		// should be backed by a single workload and it can be useful to use the workload
		// name as opposed to the label values.
		if graphType != GraphTypeVersionedApp {
			workload = ""
			version = ""
		}
		serviceName = ""
	case NodeTypeService:
		app = ""
		workload = ""
		version = ""

		if serviceName == passthroughCluster || serviceName == blackHoleCluster {
			metadata[IsEgressCluster] = true
		}
	}

	return Node{
		ID:        id,
		NodeType:  nodeType,
		Namespace: namespace,
		Workload:  workload,
		App:       app,
		Version:   version,
		Service:   serviceName,
		Edges:     []*Edge{},
		Metadata:  metadata,
	}
}

// AddEdge allocates and adds a new Edge node to the Node
func (s *Node) AddEdge(dest *Node) *Edge {
	e := NewEdge(s, dest)
	s.Edges = append(s.Edges, &e)
	return &e
}

// NewEdge allocates a new Edge node
func NewEdge(source, dest *Node) Edge {
	return Edge{
		Source:   source,
		Dest:     dest,
		Metadata: NewMetadata(),
	}
}

// NewTrafficMap allocates a new TrafficMap
func NewTrafficMap() TrafficMap {
	return make(map[string]*Node)
}

// NodeID generates the ID and NodeType given the telemetry
func NodeID(serviceNamespace, service string, workloadNamespace, workload, app, version, graphType string) (id, nodeType string) {
	return nodeID(serviceNamespace, service, false, workloadNamespace, workload, app, version, graphType)
}

// InjectedServiceNodeID generates the ID and NodeType given the injected Service telemetry. NodeType will always be NodeType.Service
func InjectedServiceNodeID(serviceNamespace, service, graphType string) (id, nodeType string) {
	return nodeID(serviceNamespace, service, false, "", "", "", "", graphType)
}

// RequestedServiceNodeID generates the ID and NodeType given the requestedService telemetry. NodeType will always be NodeType.Service
func RequestedServiceNodeID(serviceNamespace, service, graphType string) (id, nodeType string) {
	return nodeID(serviceNamespace, service, true, "", "", "", "", graphType)
}

func nodeID(serviceNamespace, service string, isRequestedService bool, workloadNamespace, workload, app, version, graphType string) (id, nodeType string) {
	// prefer the workload namespace
	namespace := workloadNamespace
	if !IsOK(namespace) {
		namespace = serviceNamespace
	}

	// first, check for the special-case "unknown" source node
	if Unknown == namespace && Unknown == workload && Unknown == app && service == "" {
		return fmt.Sprint("unknown_source"), NodeTypeUnknown
	}

	// It is possible that a request is made for an unknown destination. For example, an Ingress
	// request to an unknown path. In this case the namespace may or may not be unknown.
	// Every other field is unknown. Allow one unknown service per namespace to help reflect these
	// bad destinations in the graph,  it may help diagnose a problem.
	if Unknown == workload && Unknown == app && Unknown == service {
		return fmt.Sprintf("svc_%s_unknown", namespace), NodeTypeService
	}

	workloadOk := IsOK(workload)
	appOk := IsOK(app)
	serviceOk := IsOK(service)

	if !workloadOk && !appOk && !serviceOk {
		panic(fmt.Sprintf("Failed ID gen: namespace=[%s] workload=[%s] app=[%s] version=[%s] service=[%s] graphType=[%s]", namespace, workload, app, version, service, graphType))
	}

	svcPrefix := "svc"
	if isRequestedService {
		svcPrefix = "requested_svc"
	}
	// handle workload graph nodes (service graphs are initially processed as workload graphs)
	if graphType == GraphTypeWorkload || graphType == GraphTypeService {
		// workload graph nodes are type workload or service
		if !workloadOk && !serviceOk {
			panic(fmt.Sprintf("Failed ID gen: namespace=[%s] workload=[%s] app=[%s] version=[%s] service=[%s] graphType=[%s]", namespace, workload, app, version, service, graphType))
		}
		if !workloadOk {
			return fmt.Sprintf("%s_%v_%v", svcPrefix, namespace, service), NodeTypeService
		}
		return fmt.Sprintf("wl_%v_%v", namespace, workload), NodeTypeWorkload
	}

	// handle app and versionedApp graphs
	versionOk := IsOK(version)
	if appOk {
		// For a versionedApp graph use workload as the Id, if available. It allows us some protection
		// against labeling anti-patterns. It won't be there in a few cases like:
		//   - root node of a node graph
		//   - app box node
		// Otherwise use what we have and alter node type as necessary
		// For a [versionless] App graph use the app label to aggregate versions/workloads into one node
		if graphType == GraphTypeVersionedApp {
			if workloadOk {
				return fmt.Sprintf("vapp_%v_%v", namespace, workload), NodeTypeApp
			}
			if versionOk {
				return fmt.Sprintf("vapp_%v_%v_%v", namespace, app, version), NodeTypeApp
			}
		}
		return fmt.Sprintf("app_%v_%v", namespace, app), NodeTypeApp
	}

	// fall back to workload if applicable
	if workloadOk {
		return fmt.Sprintf("wl_%v_%v", namespace, workload), NodeTypeWorkload
	}

	// fall back to service as a last resort in the app graph
	return fmt.Sprintf("%s_%v_%v", svcPrefix, namespace, service), NodeTypeService
}
