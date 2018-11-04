package DAGAChallengeGeneration_test

import (
	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/student_18_daga/daga_login"
	"github.com/dedis/student_18_daga/daga_login/protocols/DAGAChallengeGeneration"
	protocols_testing "github.com/dedis/student_18_daga/daga_login/protocols/testing"
	"github.com/dedis/student_18_daga/sign/daga"
	"github.com/stretchr/testify/require"
	"testing"
)

var tSuite = daga.NewSuiteEC()

func TestMain(m *testing.M) {
	log.MainTest(m)
}

// Tests a 2, 5 and 13-node system. (complete protocol run)
func TestChallengeGeneration(t *testing.T) {
	nodes := []int{2, 5, 13}
	for _, nbrNodes := range nodes {
		runProtocol(t, nbrNodes)
	}
}

// TODO more DRY helpers fair share of code is .. shared..

func runProtocol(t *testing.T, nbrNodes int) {
	log.Lvl2("Running", DAGAChallengeGeneration.Name , "with", nbrNodes, "nodes")
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	services, _, dummyContext := protocols_testing.ValidServiceSetup(local, nbrNodes)

	// create and setup root protocol instance + start protocol
	challengeGeneration := services[0].(*protocols_testing.DummyService).NewDAGAChallengeGenerationProtocol(t, *dummyContext)

	challenge, err := challengeGeneration.WaitForResult()
	require.NoError(t, err, "failed to get result of protocol run")
	require.NotZero(t, challenge)

	// verify that all servers correctly signed the challenge
	// QUESTION: not sure if I should test theses here.. IMO the sut is the protocol, not the daga code it uses
	// QUESTION: and I have a daga function that is currently private that do that..
	bytes, _ := challenge.Cs.MarshalBinary()
	_, Y := dummyContext.Members()
	for _, signature := range challenge.Sigs {
		require.NoError(t, daga.SchnorrVerify(tSuite, Y[signature.Index], bytes, signature.Sig))
	}
}

func TestLeaderSetup(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	// valid setup, should not panic
	nbrNodes := 1
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, nbrNodes-1, true)
	dagaServers, _, dummyContext := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	require.NotPanics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, dagaServers[0])
	}, "should not panic on valid input")
}

func TestLeaderSetupShouldPanicOnEmptyContext(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 1
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, nbrNodes-1, true)
	dagaServers, _, _ := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(daga_login.Context{}, dagaServers[0])
	}, "should panic on empty context")
}

func TestLeaderSetupShouldPanicOnNilServer(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 1
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, nbrNodes-1, true)
	_, _, dummyContext := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, nil)
	}, "should panic on nil server")
}

func TestLeaderSetupShouldPanicOnInvalidState(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 1
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, nbrNodes-1, true)
	dagaServers, _, dummyContext := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)

	pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, dagaServers[0])
	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, dagaServers[0])
	}, "should panic on already initialized node")
	pi.(*DAGAChallengeGeneration.Protocol).Done()


	pi, _ = local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	pi.(*DAGAChallengeGeneration.Protocol).ChildrenSetup(dagaServers[0])
	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, dagaServers[0])
	}, "should panic on already initialized node")
}

func TestChildrenSetup(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	// valid setup, should not panic
	nbrNodes := 1
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, nbrNodes-1, true)
	dagaServers, _, _ := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	require.NotPanics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).ChildrenSetup(dagaServers[0])
	}, "should not panic on valid input")
}

func TestChildrenSetupShouldPanicOnNilServer(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 1
	_, _, tree := local.GenBigTree(nbrNodes, nbrNodes, nbrNodes-1, true)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).ChildrenSetup(nil)
	}, "should panic on nil server")
}

func TestChildrenSetupShouldPanicOnInvalidState(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 1
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, nbrNodes-1, true)
	dagaServers, _, dummyContext := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)

	pi.(*DAGAChallengeGeneration.Protocol).ChildrenSetup(dagaServers[0])
	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).ChildrenSetup(dagaServers[0])
	}, "should panic on already initialized node")
	pi.(*DAGAChallengeGeneration.Protocol).Done()


	pi, _ = local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, dagaServers[0])
	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).ChildrenSetup(dagaServers[0])
	}, "should panic on already initialized node")
}

func TestStartShouldErrorOnInvalidTreeShape(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 5
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, 2, true)
	dagaServers, _, dummyContext := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()
	pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, dagaServers[0])
	require.Error(t, pi.Start(), "should return error, tree has invalid shape (protocol expects that all other nodes are direct children of root)")
}
//
//func TestWaitForResultShouldErrorOnTimeout(t *testing.T) {
//	local := onet.NewLocalTest(tSuite)
//	defer local.CloseAll()
//
//
//}

func TestWaitForResultShouldPanicIfCalledBeforeStart(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 5
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, 2, true)
	dagaServers, _, dummyContext := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	pi.(*DAGAChallengeGeneration.Protocol).LeaderSetup(*dummyContext, dagaServers[0])
	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).WaitForResult()
	})
}

func TestWaitForResultShouldPanicOnNonRootInstance(t *testing.T) {
	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	nbrNodes := 5
	_, roster, tree := local.GenBigTree(nbrNodes, nbrNodes, 2, true)
	dagaServers, _, _ := protocols_testing.DummyDagaSetup(local, roster)
	pi, _ := local.CreateProtocol(DAGAChallengeGeneration.Name, tree)
	defer pi.(*DAGAChallengeGeneration.Protocol).Done()

	// TODO test name little misleading but ..

	pi.(*DAGAChallengeGeneration.Protocol).ChildrenSetup(dagaServers[0])
	require.Panics(t, func() {
		pi.(*DAGAChallengeGeneration.Protocol).WaitForResult()
	})
}

// QUESTION TODO don't know how to test more advanced things, how to simulate bad behavior from some nodes
// now I'm only assured that it works when setup like intended + some little bad things
// but no guarantees on what happens otherwise