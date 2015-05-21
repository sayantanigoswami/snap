package wmap

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/intelsdi-x/pulse/core"
	"github.com/intelsdi-x/pulse/core/cdata"
	"github.com/intelsdi-x/pulse/core/ctypes"
	"gopkg.in/yaml.v2"
)

var (
	InvalidPayload = errors.New("Payload to convert must be string or []byte")
)

func FromYaml(payload interface{}) (*WorkflowMap, error) {
	p, err := inStringBytes(payload)
	if err != nil {
		return nil, err
	}

	wmap := new(WorkflowMap)
	err = yaml.Unmarshal(p, wmap)
	if err != nil {
		return nil, err
	}
	return wmap, nil
}

func FromJson(payload interface{}) (*WorkflowMap, error) {
	p, err := inStringBytes(payload)
	if err != nil {
		return nil, err
	}

	wmap := new(WorkflowMap)
	err = json.Unmarshal(p, wmap)
	if err != nil {
		return nil, err
	}
	return wmap, nil
}

func inStringBytes(payload interface{}) ([]byte, error) {
	var p []byte
	switch tp := payload.(type) {
	case string:
		p = []byte(tp)
	case []byte:
		p = tp
	default:
		return p, InvalidPayload
	}
	return p, nil
}

func SampleWorkflowMapJson() string {
	wf := sample()
	b, e := wf.ToJson()
	if e != nil {
		panic(e)
	}
	return string(b)
}

func SampleWorkflowMapYaml() string {
	wf := sample()
	b, e := wf.ToYaml()
	if e != nil {
		panic(e)
	}
	return string(b)
}

func sample() *WorkflowMap {
	wf := new(WorkflowMap)

	c1 := &CollectWorkflowMapNode{
		Metrics: make(map[string]metricInfo),
		Config:  make(map[string]map[string]interface{}),
	}
	c1.Config["/foo/bar"] = make(map[string]interface{})
	c1.Config["/foo/bar"]["user"] = "root"

	// pr1 := &ProcessWorkflowMapNode{Name: "learn", Version: 3}
	pu1 := &PublishWorkflowMapNode{
		Name:    "rabbitmq",
		Version: 5,
		Config:  make(map[string]interface{}),
	}

	pu1.Config["user"] = "root"
	var e error
	// e = pr1.Add(pu1)
	// handleErr(e)
	e = c1.Add(pu1)
	if e != nil {
		panic(e)
	}
	e = c1.AddMetric("/foo/bar", 1)
	if e != nil {
		panic(e)
	}
	wf.CollectNode = c1
	return wf
}

// A map of a desired workflow that is used to create a scheduleWorkflow
type WorkflowMap struct {
	CollectNode *CollectWorkflowMapNode `json:"collect"yaml:"collect"`
}

func NewWorkflowMap() *WorkflowMap {
	w := &WorkflowMap{}
	c := &CollectWorkflowMapNode{
		Metrics: make(map[string]metricInfo),
		Config:  make(map[string]map[string]interface{}),
	}
	w.CollectNode = c
	return w
}

func (w *WorkflowMap) ToJson() ([]byte, error) {
	return json.Marshal(w)
}

func (w *WorkflowMap) ToYaml() ([]byte, error) {
	return yaml.Marshal(w)
}

type CollectWorkflowMapNode struct {
	Metrics      map[string]metricInfo             `json:"metrics"yaml:"metrics"`
	Config       map[string]map[string]interface{} `json:"config"yaml:"config"`
	ProcessNodes []ProcessWorkflowMapNode          `json:"process"yaml:"process"`
	PublishNodes []PublishWorkflowMapNode          `json:"publish"yaml:"publish"`
}

func (c *CollectWorkflowMapNode) GetRequestedMetrics() []core.RequestedMetric {
	var metrics []core.RequestedMetric = make([]core.RequestedMetric, len(c.Metrics))
	x := 0
	for k, v := range c.Metrics {
		metrics[x] = metric{
			namespace: strings.Split(k, "/"),
			version:   v.Version_,
		}
		x++
	}
	return metrics
}

// GetConfigTree converts config data for collection node in wmap into a proper cdata.ConfigDataTree
func (c *CollectWorkflowMapNode) GetConfigTree() (*cdata.ConfigDataTree, error) {
	cdt := cdata.NewTree()
	// Iterate over config and attempt to convert into data nodes in the tree
	for ns_, cmap := range c.Config {

		// Attempt to convert namespace string to proper namespace
		if !isValidNamespaceString(ns_) {
			return nil, errors.New(fmt.Sprintf("Invalid namespace: ", ns_))
		}
		ns := strings.Split(ns_, "/")[1:]
		cdn, err := configtoConfigDataNode(cmap, ns_)
		if err != nil {
			return nil, err
		}
		cdt.Add(ns, cdn)
	}
	return cdt, nil
}

func (c *CollectWorkflowMapNode) Add(node interface{}) error {
	switch x := node.(type) {
	case *ProcessWorkflowMapNode:
		c.ProcessNodes = append(c.ProcessNodes, *x)
	case *PublishWorkflowMapNode:
		c.PublishNodes = append(c.PublishNodes, *x)
	default:
		return errors.New(fmt.Sprintf("cannot add workflow node type (%v) to collect node as child", x))
	}
	return nil
}

func (c *CollectWorkflowMapNode) AddMetric(ns string, v int) error {
	// TODO regex validation here that this matches /one/two/three format
	// c.MetricsNamespaces = append(c.MetricsNamespaces, ns)
	c.Metrics[ns] = metricInfo{Version_: v}
	return nil
}

func (c *CollectWorkflowMapNode) AddConfigItem(ns, key string, value interface{}) {
	if c.Config[ns] == nil {
		c.Config[ns] = make(map[string]interface{})
	}
	c.Config[ns][key] = value
}

type ProcessWorkflowMapNode struct {
	Name         string                   `json:"plugin_name"yaml:"plugin_name"`
	Version      int                      `json:"plugin_version"yaml:"plugin_version"`
	ProcessNodes []ProcessWorkflowMapNode `json:"process"yaml:"process"`
	PublishNodes []PublishWorkflowMapNode `json:"publish"yaml:"publish"`
	// TODO processor config
	Config map[string]interface{} `json:"config"yaml:"config"`
}

func NewProcessNode(name string, version int) *ProcessWorkflowMapNode {
	p := &ProcessWorkflowMapNode{
		Name:    name,
		Version: version,
	}
	return p
}

func (p *ProcessWorkflowMapNode) Add(node interface{}) error {
	switch x := node.(type) {
	case *ProcessWorkflowMapNode:
		p.ProcessNodes = append(p.ProcessNodes, *x)
	case *PublishWorkflowMapNode:
		p.PublishNodes = append(p.PublishNodes, *x)
	default:
		return errors.New(fmt.Sprintf("cannot add workflow node type (%v) to process node as child", x))
	}
	return nil
}

func (p *ProcessWorkflowMapNode) AddConfigItem(key string, value interface{}) {
	if p.Config == nil {
		p.Config = make(map[string]interface{})
	}
	p.Config[key] = value
}

func (p *ProcessWorkflowMapNode) GetConfigNode() (*cdata.ConfigDataNode, error) {
	if p.Config == nil {
		return cdata.NewNode(), nil
	}
	return configtoConfigDataNode(p.Config, "")
}

type PublishWorkflowMapNode struct {
	Name    string `json:"plugin_name"yaml:"plugin_name"`
	Version int    `json:"plugin_version"yaml:"plugin_version"`
	// TODO publisher config
	Config map[string]interface{} `json:"config"yaml:"config"`
}

func NewPublishNode(name string, version int) *PublishWorkflowMapNode {
	p := &PublishWorkflowMapNode{
		Name:    name,
		Version: version,
	}
	return p
}

func (p *PublishWorkflowMapNode) AddConfigItem(key string, value interface{}) {
	if p.Config == nil {
		p.Config = make(map[string]interface{})
	}
	p.Config[key] = value
}

func (p *PublishWorkflowMapNode) GetConfigNode() (*cdata.ConfigDataNode, error) {
	if p.Config == nil {
		return cdata.NewNode(), nil
	}
	return configtoConfigDataNode(p.Config, "")
}

type metricInfo struct {
	Version_ int `json:"version"yaml:"version"`
}

type metric struct {
	namespace []string
	version   int
}

func (m metric) Namespace() []string {
	return m.namespace
}

func (m metric) Version() int {
	return m.version
}

func isValidNamespaceString(ns string) bool {
	b, err := regexp.MatchString("^(/[a-z0-9]+)+$", ns)
	if err != nil {
		// Just safety in case regexp packages changes in some way to break this in the future.
		panic(err)
	}
	return b
}

func configtoConfigDataNode(cmap map[string]interface{}, ns string) (*cdata.ConfigDataNode, error) {
	cdn := cdata.NewNode()
	for ck, cv := range cmap {
		switch v := cv.(type) {
		case string:
			cdn.AddItem(ck, ctypes.ConfigValueStr{Value: v})
		case int:
			cdn.AddItem(ck, ctypes.ConfigValueInt{Value: v})
		case float64:
			cdn.AddItem(ck, ctypes.ConfigValueFloat{Value: v})
		case bool:
			cdn.AddItem(ck, ctypes.ConfigValueBool{Value: v})
		default:
			// TODO make sure this is covered in tests!!!
			return nil, errors.New(fmt.Sprintf("Cannot convert config value to config data node: %s=>%+v", ns, v))
		}
	}
	return cdn, nil
}
