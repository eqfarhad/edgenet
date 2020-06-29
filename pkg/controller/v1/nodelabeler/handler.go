package nodelabeler

import (
	"edgenet/pkg/client/clientset/versioned"
	"edgenet/pkg/node"

	log "github.com/Sirupsen/logrus"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// HandlerInterface interface contains the methods that are required
type HandlerInterface interface {
	Init(kubernetes kubernetes.Interface, edgenet versioned.Interface) error
	SetNodeGeolocation(obj interface{})
}

// Handler is a sample implementation of Handler
type Handler struct {
	clientset        kubernetes.Interface
	edgenetClientset versioned.Interface
}

// Init handles any handler initialization
func (t *Handler) Init(kubernetes kubernetes.Interface, edgenet versioned.Interface) error {
	t.clientset = kubernetes
	t.edgenetClientset = edgenet
	log.Info("Handler.Init")
	return nil
}

// SetNodeGeolocation is called when an object is created or updated
func (t *Handler) SetNodeGeolocation(obj interface{}) {
	log.Info("Handler.ObjectCreated")
	// Get internal and external IP addresses of the node
	internalIP, externalIP := node.GetNodeIPAddresses(obj.(*api_v1.Node))
	result := false
	// Check if the external IP exists to use it in the first place
	if externalIP != "" {
		log.Infof("External IP: %s", externalIP)
		result, _ = node.GetGeolocationByIP(obj.(*api_v1.Node).Name, externalIP, t.clientset)
	}
	// Check if the internal IP exists and
	// the result of detecting geolocation by external IP is false
	if internalIP != "" && result == false {
		log.Infof("Internal IP: %s", internalIP)
		node.GetGeolocationByIP(obj.(*api_v1.Node).Name, internalIP, t.clientset)
	}
}
