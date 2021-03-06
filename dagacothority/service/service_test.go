package service

import (
	"go.dedis.ch/kyber"
	"go.dedis.ch/kyber/util/key"
	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"github.com/dedis/student_18_daga/dagacothority"
	testing2 "github.com/dedis/student_18_daga/dagacothority/testing"
	"github.com/dedis/student_18_daga/sign/daga"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

// TODO create helpers that build various requests, and have the test that test API endpoints accept request as parameter

var tSuite = daga.NewSuiteEC()

func TestMain(m *testing.M) {
	log.MainTest(m, 4)
}

// populate services state/storage with the dagaServers and context provided
func populateServicesStates(services []onet.Service, dagaServers []daga.Server, context *dagacothority.Context) {
	for i, s := range services {
		// plug some initial test state: (in real life those are (for now) fetched from FS during setupState)
		svc := s.(*Service)
		dagaServer := dagaServers[i]
		svc.PopulateServiceState(context, dagaServer)
	}
}

// verify that CreateContext call succeed on valid request
func TestService_CreateContext(t *testing.T) {
	local := onet.NewTCPTest(tSuite)
	hosts, roster, _ := local.GenTree(5, true)
	defer local.CloseAll()

	services := local.GetServices(hosts, DagaID)

	// override Setup
	unusedServers := make([]daga.Server, 0, len(services))
	for range services {
		dagaServer, _ := daga.NewServer(tSuite, 0, nil)
		unusedServers = append(unusedServers, dagaServer)
	}
	unusedContext := dagacothority.Context{}
	populateServicesStates(services, unusedServers, &unusedContext)

	for _, s := range services {
		log.Lvl2("Sending request to", s)

		// create valid request
		subscriberKeys := testing2.RandomPointSlice(32)
		request := dagacothority.CreateContext{
			ServiceID:       dagacothority.ServiceID(uuid.Must(uuid.NewV4())),
			DagaNodes:       roster,
			SubscribersKeys: subscriberKeys,
		}

		// TODO use openPGP or whatever (now there is no verification, here only to documente one of the options I had in mind)
		keyPair := key.NewKeyPair(tSuite)
		hasher := tSuite.Hash()
		hasher.Write(uuid.UUID(request.ServiceID).Bytes())
		pointBytes, err := daga.PointArrayToBytes(request.SubscribersKeys)
		require.NoError(t, err)
		hasher.Write(pointBytes)
		signature, err := daga.SchnorrSign(tSuite, keyPair.Private, hasher.Sum(nil))
		request.Signature = signature

		reply, err := s.(*Service).CreateContext(&request)
		require.NoError(t, err)
		require.NotZero(t, reply)
		require.NotZero(t, reply.Context)

		// verify correctness ...
		context := reply.Context
		members := context.Members()
		contextBytes, err := daga.AuthenticationContextToBytes(context) // TODO see to include other things (roster Ids etc..)
		require.NoError(t, err)
		for i, pubKey := range members.Y {
			require.NoError(t, daga.SchnorrVerify(tSuite, pubKey, contextBytes, context.Signatures[i]))
		}
		require.True(t, dagacothority.ContainsSameElems(subscriberKeys, context.Members().X))
	}
}

// verify that PKClient call succeed on valid request
func TestService_PKClient(t *testing.T) {
	local := onet.NewTCPTest(tSuite) // QUESTION: vs localTest ?
	hosts, roster, _ := local.GenTree(5, true)
	defer local.CloseAll()

	services := local.GetServices(hosts, DagaID)
	_, dagaServers, _, dummyContext := testing2.DummyDagaSetup(rand.Intn(10)+2, len(local.Servers), roster)

	// provide initial state to the service (instead of fetching it from FS)
	populateServicesStates(services, dagaServers, dummyContext)

	for _, s := range services { // QUESTION purpose of running test on multiple "same" service ??
		log.Lvl2("Sending request to", s)

		commitments := testing2.RandomPointSlice(len(dummyContext.ClientsGenerators()) * 3)

		reply, err := s.(*Service).PKClient(
			&dagacothority.PKclientCommitments{
				Context:     *dummyContext,
				Commitments: commitments},
		)
		require.NoError(t, err)
		require.NotZero(t, reply)

		// verify that all servers correctly signed the challenge
		members := dummyContext.Members()
		require.NoError(t, reply.NetDecode().VerifySignatures(tSuite, members.Y, commitments))
	}
}

// verify that Auth call succeed on valid request
func TestService_Auth(t *testing.T) {
	local := onet.NewTCPTest(tSuite) // QUESTION: vs localTest ?
	hosts, roster, _ := local.GenTree(5, true)
	defer local.CloseAll()

	services := local.GetServices(hosts, DagaID)
	_, dagaServers, dummyAuthRequest, dummyContext := testing2.DummyDagaSetup(rand.Intn(10)+2, len(local.Servers), roster)

	// provide initial state to the service (instead of fetching it from FS)
	populateServicesStates(services, dagaServers, dummyContext)

	for _, s := range services {
		log.Lvl2("Sending request to", s)

		request := dagacothority.Auth(*dagacothority.NetEncodeAuthenticationMessage(*dummyContext, *dummyAuthRequest))
		reply, err := s.(*Service).Auth(&request)
		require.NoError(t, err)
		require.NotZero(t, reply)

		serverMsg, context := reply.NetDecode()
		require.True(t, context.Equals(*dummyContext), "context part of reply different than context of request")
		// verify / extract tag
		Tf, err := daga.GetFinalLinkageTag(tSuite, dummyContext, *serverMsg)
		require.NoError(t, err, "failed to extract tag from the resulting serverMsg")
		require.NotZero(t, Tf)
	}
}

// verify that PKClient works for context created with CreateContext
// TODO understand why this one is fine on my machine (with race flag too) but "FAIL still have things lingering" in travis
//  seems that in travis the test is stopped prematurely .. some timeout ?
//  => no, context not found in state but that's not possible..context is obtained/returned by service only after successful storage in state...
func TestService_CreateContextAndPKClient(t *testing.T) {
	local := onet.NewTCPTest(tSuite)
	hosts, roster, _ := local.GenTree(5, true)
	defer local.CloseAll()

	services := local.GetServices(hosts, DagaID)

	for _, s := range services {

		// calls CreateContext
		context, _ := getTestContext(t, s.(*Service), roster, 5)
		
		commitments := testing2.RandomPointSlice(len(context.ClientsGenerators()) * 3)
		reply, err := s.(*Service).PKClient(
			&dagacothority.PKclientCommitments{
				Context:     context,
				Commitments: commitments},
		)
		require.NoError(t, err)
		require.NotZero(t, reply)

		// verify that all servers correctly signed the challenge
		members := context.Members()
		require.NoError(t, reply.NetDecode().VerifySignatures(tSuite, members.Y, commitments))
	}
}

// retrieve a test context created by calling the CreateContext endpoint with dummy parameters, to use it in other tests
func getTestContext(t *testing.T, s *Service, roster *onet.Roster, numClients int) (dagacothority.Context, []daga.Client) {

	clients := make([]daga.Client, numClients)
	keys := make([]kyber.Point, 0, numClients)
	for i := range clients {
		client, err := daga.NewClient(tSuite, i, nil)
		require.NoError(t, err)
		clients[i] = client
		keys = append(keys, client.PublicKey())
	}

	createContextRequest := dagacothority.CreateContext{
		Signature:       make([]byte, 32),
		DagaNodes:       roster,
		SubscribersKeys: keys,
		ServiceID:       dagacothority.ServiceID(uuid.Must(uuid.NewV4())),
	}
	createContextReply, err := s.CreateContext(&createContextRequest)
	require.NoError(t, err)
	require.NotZero(t, createContextReply)
	require.NotZero(t, createContextReply.Context)

	return createContextReply.Context, clients
}

// verify that Auth works for context created with CreateContext and Challenge received from PKClient, i.e: "full test"
func TestService_CreateContextAndPKclientAndAuth(t *testing.T) {
	local := onet.NewTCPTest(tSuite)
	hosts, roster, _ := local.GenTree(5, true)
	defer local.CloseAll()

	for _, s := range local.GetServices(hosts, DagaID) {
		log.Lvl2("Sending request to", s)

		// calls CreateContext
		context, clients := getTestContext(t, s.(*Service), roster, 5)

		// calls PKClient to build the auth. message
		authMsg, err := daga.NewAuthenticationMessage(tSuite, context, clients[0], func(commits []kyber.Point) (daga.Challenge, error) {
			request := dagacothority.PKclientCommitments{
				Commitments: commits,
				Context:     context,
			}
			reply, err := s.(*Service).PKClient(&request)
			require.NoError(t, err)
			return *reply.NetDecode(), nil
		})

		// calls Auth
		authRequest := dagacothority.Auth(*dagacothority.NetEncodeAuthenticationMessage(context, *authMsg))
		authReply, err := s.(*Service).Auth(&authRequest)
		require.NoError(t, err)
		require.NotZero(t, authReply)

		serverMsg, context := authReply.NetDecode()
		require.NoError(t, err)
		require.True(t, context.Equals(context), "context part of reply different than context of request")

		// verify / extract tag
		Tf, err := daga.GetFinalLinkageTag(tSuite, context, *serverMsg)
		require.NoError(t, err, "failed to extract tag from the resulting serverMsg")
		require.NotZero(t, Tf)
	}
}

func TestValidateAuthReqShouldErrorOnNilReq(t *testing.T) {
	service := &Service{}
	context, err := service.validateAuthReq(nil)
	require.Error(t, err, "should return error on nil req")
	require.Zero(t, context)
}

func TestValidateAuthReqShouldErrorOnEmptyReq(t *testing.T) {
	service := &Service{}
	context, err := service.validateAuthReq(&dagacothority.Auth{
		Context:  dagacothority.Context{},
		T0:       nil,
		SCommits: nil,
		Proof:    dagacothority.ClientProof{},
	})
	require.Error(t, err, "should return error on empty req")
	require.Zero(t, context)
}

func TestValidateContextShouldErrorOnInvalidContext(t *testing.T) {
	local := onet.NewTCPTest(tSuite)
	hosts, roster, _ := local.GenTree(2, true)
	defer local.CloseAll()
	services := local.GetServices(hosts, DagaID)
	service := services[0].(*Service)

	badContext := dagacothority.Context{
		Roster: roster,

		X: testing2.RandomPointSlice(5),
		Y: testing2.RandomPointSlice(9),
		H: testing2.RandomPointSlice(3), // len != 5 => invalid
		R: testing2.RandomPointSlice(8), // len != 9 => invalid

		ServiceID: dagacothority.ServiceID(uuid.Must(uuid.NewV4())),
		ContextID: dagacothority.ContextID(uuid.Must(uuid.NewV4())),
	}

	context, err := service.validateContext(badContext)
	require.Error(t, err, "should return error on invalid context")
	require.Zero(t, context)
}

func TestValidateContextShouldErrorOnEmptyRoster(t *testing.T) {
	service := &Service{}
	badContext := dagacothority.Context{
		Roster: &onet.Roster{},

		X: testing2.RandomPointSlice(5),
		Y: testing2.RandomPointSlice(9),
		H: testing2.RandomPointSlice(5),
		R: testing2.RandomPointSlice(9),

		ServiceID: dagacothority.ServiceID(uuid.Must(uuid.NewV4())),
		ContextID: dagacothority.ContextID(uuid.Must(uuid.NewV4())),
	}
	context, err := service.validateContext(badContext)
	require.Error(t, err, "should return error on empty roster")
	require.Zero(t, context)

	badContext.Roster = &onet.Roster{List: make([]*network.ServerIdentity, 0, 5)}
	context, err = service.validateContext(badContext)
	require.Error(t, err, "should return error on empty roster")
	require.Zero(t, context)
}

func TestValidateContextShouldErrorOnUnacceptedContext(t *testing.T) {
	local := onet.NewTCPTest(tSuite)
	hosts, roster, _ := local.GenTree(5, true)
	defer local.CloseAll()

	services := local.GetServices(hosts, DagaID)
	_, dagaServers, _, dummyContext := testing2.DummyDagaSetup(rand.Intn(10)+2, len(local.Servers), roster)
	// provide initial state to the service (instead of fetching it from FS)
	populateServicesStates(services[0:0], dagaServers, dummyContext)
	service := services[0].(*Service)

	// same roster but bullshit in daga.Context
	badNetContext := dagacothority.Context{
		Roster: roster,

		X: testing2.RandomPointSlice(5),
		Y: testing2.RandomPointSlice(9),
		H: testing2.RandomPointSlice(5),
		R: testing2.RandomPointSlice(9),

		ServiceID: dagacothority.ServiceID(uuid.Must(uuid.NewV4())),
		ContextID: dagacothority.ContextID(uuid.Must(uuid.NewV4())),
	}
	context, err := service.validateContext(badNetContext)
	require.Error(t, err, "should return error on not accepted context")
	require.Zero(t, context)
}

func TestValidatePKClientReqShouldErrorOnNilRequest(t *testing.T) {
	service := &Service{}

	context, err := service.ValidatePKClientReq(nil)
	require.Error(t, err, "should return error on nil request")
	require.Zero(t, context)
}

func TestValidatePKClientReqShouldErrorOnEmptyOrBadlySizedCommitments(t *testing.T) {
	service := &Service{}

	context, err := service.ValidatePKClientReq(&dagacothority.PKclientCommitments{
		Context: dagacothority.Context{},
	})
	require.Error(t, err, "should return error on empty request")
	require.Zero(t, context)

	context, err = service.ValidatePKClientReq(&dagacothority.PKclientCommitments{
		Context: dagacothority.Context{
			H: testing2.RandomPointSlice(8),
		},
		Commitments: testing2.RandomPointSlice(12), // != 3*8
	})
	require.Error(t, err, "should return error on bad commitments size")
	require.Zero(t, context)
}
