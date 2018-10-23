package service

/*
The service.go defines what to do for each API-call. This part of the service
runs on the node.
*/

import (
	"errors"
	"fmt"
	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"github.com/dedis/student_18_daga/daga_login"
	"github.com/dedis/student_18_daga/sign/daga"
	"sync"
)

// Used for tests
var templateID onet.ServiceID

func init() {
	var err error
	templateID, err = onet.RegisterNewService(daga_login.ServiceName, newService)
	log.ErrFatal(err)
	network.RegisterMessages(storage{}, daga_login.NetContext{})
}

// Service is our template-service // TODO
type Service struct {
	// We need to embed the ServiceProcessor, so that incoming messages
	// are correctly handled.
	*onet.ServiceProcessor

	storage *storage
}

// TODO load auth context from well defined place!

// storageID reflects the data we're storing - we could store more
// than one structure.
var storageID = []byte("main")

// storage is used to save our data.
type storage struct {
	Context daga_login.NetContext
	Count int
	sync.Mutex
}

//// Clock starts a template-protocol and returns the run-time.
//func (s *Service) Clock(req *daga_login.Clock) (*daga_login.ClockReply, error) {
//	s.storage.Lock()
//	s.storage.Count++
//	s.storage.Unlock()
//	s.save()
//	tree := req.Roster.GenerateNaryTreeWithRoot(2, s.ServerIdentity())
//	if tree == nil {
//		return nil, errors.New("couldn't create tree")
//	}
//	pi, err := s.CreateProtocol(protocol.Name, tree)
//	if err != nil {
//		return nil, err
//	}
//	start := time.Now()
//	pi.Start()
//	resp := &daga_login.ClockReply{  // FIXME not called when #nodes = 1 ..(why ?)
//		Children: <-pi.(*protocol.TemplateProtocol).ChildCount,
//	}
//	resp.Time = time.Now().Sub(start).Seconds()
//	return resp, nil
//}

// Login starts the server's protocol (daga 4.3.6)
func (s *Service) Login(req *daga.AuthenticationMessage) (*daga.ServerMessage, error) {
	context, err := s.storage.Context.NetDecode()
	if err != nil {
		return nil, err
	}
	// TODO ring
	// TODO will need a mapping from public keys to serveridentity/address
	return nil, fmt.Errorf("unimplemented, %v", context)
}

// PKClient starts the challenge generation protocol
func (s *Service) PKClient(req *daga_login.PKclientCommitments) (*daga_login.PKclientChallenge, error) {
	dummyChallenge := daga.Challenge{
		Cs:daga.NewSuiteEC().Scalar().SetInt64(42),
	}

	// QUESTION current conode kind of act as a RandHerd proxy => maybe bad idea.. If we use randherd maybe client/prover can request
	// new randomness and since it is public, the servers can verify/accept the randomness / challenge only if it is timestamped after the recception of the commitments => prover cannot cheat
	//randClient := randhound.NewClient()
	//reply, err := randClient.Random(s.Context, c.Int("index"))
	//if err != nil {
	//	return err
	//}
	// and keep in mind of the daga traditional way to generate challenge

	log.Lvl1("dfdsfsdfssdf")
	return (*daga_login.PKclientChallenge)(&dummyChallenge), nil
	//return nil, fmt.Errorf("not implemented")//, but here is what I received: %v", *req)
}


// QUESTION: don't understand
// NewProtocol is called on all nodes of a Tree (except the root, since it is
// the one starting the protocol) so it's the Service that will be called to
// generate the PI on all others node.
// If you use CreateProtocolOnet, this will not be called, as the Onet will
// instantiate the protocol on its own. If you need more control at the
// instantiation of the protocol, use CreateProtocolService, and you can
// give some extra-configuration to your protocol in here.
func (s *Service) NewProtocol(tn *onet.TreeNodeInstance, conf *onet.GenericConfig) (onet.ProtocolInstance, error) {
	log.Lvl3("Not templated yet")
	return nil, nil
}

// saves all data.
func (s *Service) save() {
	s.storage.Lock()
	defer s.storage.Unlock()
	err := s.Save(storageID, s.storage)
	if err != nil {
		log.Error("Couldn't save data:", err)
	}
}

// Tries to load the configuration and updates the data in the service
// if it finds a valid config-file
func (s *Service) tryLoad() error {
	s.storage = &storage{}
	msg, err := s.Load(storageID)
	if err != nil {
		return err
	}
	if msg == nil {
		// first time or nothing, load from setup files FIXME temp hack
		context, err := daga_login.ReadContext("./context.bin")
		if err != nil {
			return errors.New("tryLoad: first run, failed to read context from config file: " + err.Error())
		}
		netContext := daga_login.NetEncodeContext(context)
		s.storage.Context = *netContext
		return nil
	} else {
		var ok bool
		s.storage, ok = msg.(*storage)
		if !ok {
			return errors.New("tryLoad: data of wrong type")
		}
		return nil
	}
}

// newService receives the context that holds information about the node it's
// running on. Saving and loading can be done using the context. The data will
// be stored in memory for tests and simulations, and on disk for real deployments.
func newService(c *onet.Context) (onet.Service, error) {
	s := &Service{
		ServiceProcessor: onet.NewServiceProcessor(c),
	}
	if err := s.RegisterHandlers(s.Login, s.PKClient); err != nil {
		return nil, errors.New("Couldn't register messages: " + err.Error())
	}
	if err := s.tryLoad(); err != nil {
		log.Error(err)
		return nil, err
	}
	return s, nil
}
