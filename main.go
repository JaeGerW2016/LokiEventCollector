package main

import (
	"LokiEventCollector/config"
	"LokiEventCollector/controller"
	"context"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var configFile string
var LeaderElect bool

const component = "loki-event-collector"
const inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

func init() {
	flag.StringVar(&configFile, "c", "", "config file to load,default path: ./config.json -> /etc/loki-event-collector/config.json -> /$HOME/.config/loki-event-collector/config.json")
	flag.BoolVar(&LeaderElect, "le", false, "enable leader election mode, default: false  use standalone mode")
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339Nano,
	})

	config.InitConfig(configFile)
}

func getKubeClient() (*kubernetes.Clientset, error) {
	var c *rest.Config
	c, err := rest.InClusterConfig()
	if err != nil && err == rest.ErrNotInCluster {
		c, err = clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(c)
}

func getInClusterNamespace() (string, error) {
	_, err := os.Stat(inClusterNamespacePath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("not running incluster, please specify leaderElectionIDspace")
	} else if err != nil {
		return "", fmt.Errorf("error checking namespace file: %v", err)
	}
	namespace, err := ioutil.ReadFile(inClusterNamespacePath)
	if err != nil {
		return "", fmt.Errorf("error reading namespace file: %v", err)
	}
	return string(namespace), nil
}

func main() {
	cs, err := getKubeClient()
	if err != nil {
		logrus.Fatal(err)
	}

	stop := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		sig := <-sigs
		logrus.Println("receive signal", sig)
		close(stop)
	}()

	run := func(ctx context.Context) {
		ec := controller.NewEventController(cs)
		ec.Run(10, stop)
	}

	if !LeaderElect {
		logrus.Infof("start LokiEventCollector with stand-alone mode")
		run(ctx)
		logrus.Fatal("exited")
	}

	var id string
	var leaseLockName = component
	var leaseLockNamespace string
	id, err = os.Hostname()
	if err != nil {
		logrus.Fatalf("get hostname error: %v", err)
	}

	id = id + "_" + string(uuid.NewUUID())
	leaseLockNamespace, err = getInClusterNamespace()
	if err != nil {
		leaseLockNamespace = corev1.NamespaceDefault
	}
	logrus.Infof("start LokiEventCollector with leader-election mode, Id: %s", id)
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Namespace: leaseLockNamespace,
			Name:      leaseLockName,
		},
		Client: cs.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				logrus.Infof("start leading: %s", id)
				run(ctx)
			},
			OnStoppedLeading: func() {
				logrus.Fatalf("leaderelection lost: %s", id)
			},
		},
		ReleaseOnCancel: true,
		Name:            component,
	})
	logrus.Println("exited")
}
