package controller

import (
	"LokiEventCollector/config"
	"LokiEventCollector/receiver"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"time"
)

type EventController struct {
	clientset         kubernetes.Interface
	informerFactory   informers.SharedInformerFactory
	eventInformer     coreinformers.EventInformer
	stopper           chan struct{}
	informerHasSynced bool
	targets           []receiver.Receiver
	queue             workqueue.Interface
}

func (ec *EventController) enqueue(e *corev1.Event, handleType string) {
	logrus.Infof("kube event [%s] %s [%s][%s/%s][%s] uid: [%s] last since %v", e.Type, e.Reason, e.InvolvedObject.Namespace, e.InvolvedObject.Kind, e.InvolvedObject.Name, e.Message, e.InvolvedObject.UID, time.Since(e.LastTimestamp.Time))
	// prevent old events being handled when controller just start
	if time.Since(e.LastTimestamp.Time) > time.Second*5 {
		return
	}
	logrus.Infof("event to send [%s]", string(e.UID))
	ec.queue.Add(e)
}
func (ec *EventController) worker() {
	for ec.processNextItem() {
	}
}

func (ec *EventController) processNextItem() bool {
	item, quit := ec.queue.Get()
	if quit {
		return false
	}
	event := item.(*corev1.Event)
	defer ec.queue.Done(item)

	for _, t := range ec.targets {
		if !t.Filter(event) {
			continue
		}
		err := t.Send(event)
		if err != nil {
			logrus.Errorf("send event to [%s] error: %v", t.Name(), err)
		}
	}
	return true
}

func (ec *EventController) Run(workers int, stop <-chan struct{}) {
	logrus.Info("starting event controller")
	defer logrus.Info("stopping event controller")
	defer ec.queue.ShutDown()
	ec.informerFactory.Start(stop)
	if !cache.WaitForCacheSync(stop, ec.eventInformer.Informer().HasSynced) {
		logrus.Error("wait for cache sync error")
		return
	}
	ec.informerHasSynced = true
	logrus.Info("informer cache synced, controller started")
	for i := 0; i < workers; i++ {
		go wait.Until(ec.worker, time.Second, stop)
	}
	<-stop
}

func (ec *EventController) addHandlers() {
	ec.eventInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			e := obj.(*corev1.Event)
			ec.enqueue(e, "ADD")
		},
		UpdateFunc: func(_, newObj interface{}) {
			e := newObj.(*corev1.Event)
			ec.enqueue(e, "UPDATE")
		},
		DeleteFunc: func(obj interface{}) {
			// Nothing to do
		},
	})
}

func (ec *EventController) Stop() {
	close(ec.stopper)
}

func NewEventController(cs *kubernetes.Clientset) *EventController {
	factory := informers.NewSharedInformerFactory(cs, 0)
	eventInformer := factory.Core().V1().Events()

	// loki / stdout
	var targets []receiver.Receiver
	if config.C.Receivers.Loki != nil {
		target, err := receiver.NewLokiTarget(config.C.Receivers.Loki)
		if err != nil {
			logrus.Errorf("create receiver error: %v", err)
		} else {
			targets = append(targets, target)
			logrus.Infof("receiver loaded, %s", target.Name())
		}
	}
	if config.C.Receivers.Stdout {
		target, _ := receiver.NewStdoutTarget()
		targets = append(targets, target)
		logrus.Infof("receiver loaded, %s", target.Name())
	}

	ec := &EventController{
		clientset:       cs,
		informerFactory: factory,
		eventInformer:   eventInformer,
		stopper:         make(chan struct{}),
		targets:         targets,
		queue:           workqueue.New(),
	}
	ec.addHandlers()

	return ec
}
