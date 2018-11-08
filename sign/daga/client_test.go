package daga

import (
	"errors"
	"github.com/dedis/kyber"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

var suite = NewSuiteEC()

// FIXME review/see if the test are sound and were correctly written
// FIXME and change the require.Error calls into require.EqualError to assert that it is indeed the expected error fired and not another error that could hide a bug..
func TestNewClient(t *testing.T) {
	//Normal execution
	i := rand.Int()
	s := suite.Scalar().Pick(suite.RandomStream())
	client, err := NewClient(suite, i, s)
	require.NoError(t, err, "Cannot initialize a new client with a given private key")
	require.Equal(t, i, client.Index(), "Cannot initialize a new client with a given private key, wrong index")
	require.True(t, client.PrivateKey().Equal(s), "Cannot initialize a new client with a given private key, wrong key")

	client, err = NewClient(suite, i, nil)
	require.NoError(t, err, "Cannot create a new client without a private key")

	//Invalid input
	client, err = NewClient(suite, -2, s)
	require.Error(t, err, "Wrong check: Invalid index")
}

func TestNewInitialTagAndCommitments(t *testing.T) {
	clients, servers, context, _ := generateTestContext(suite, rand.Intn(10)+2, rand.Intn(10)+2)

	// normal execution
	_, Y := context.Members()
	tagAndCommitments, s := newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])
	T0, S := tagAndCommitments.T0, tagAndCommitments.SCommits
	require.NotNil(t, T0, "T0 nil")
	require.NotNil(t, S, "sCommits nil")
	require.NotNil(t, s, "s nil")
	require.False(t, T0.Equal(suite.Point().Null()), "T0 is the null point")
	require.Equal(t, len(S), len(servers)+2, "S has the wrong length: %d instead of %d", len(S), len(servers)+2)
	for i, temp := range S {
		require.False(t, temp.Equal(suite.Point().Null()), "Null point in sCommits at position %d", i)
	}
}

// test helper that returns a properly signed Challenge by signing cs||pkClientCommitments using the keys of the servers
func signDummyChallenge(cs kyber.Scalar, servers []Server, pkClientCommitments []kyber.Point) Challenge {
	challenge := Challenge{Cs: cs}
	signData, _ := challenge.ToBytes(pkClientCommitments)
	var sigs []ServerSignature
	//Make each test server sign the challenge
	for _, server := range servers {
		sig, _ := SchnorrSign(suite, server.PrivateKey(), signData)
		sigs = append(sigs, ServerSignature{Index: server.Index(), Sig: sig})
	}
	challenge.Sigs = sigs
	return challenge
}

// test helper that returns dummy "channel" to act as a dummy server/verifier
// that return challenge upon reception of the prover's commitments
func newDummyServerChannels(cs kyber.Scalar, servers []Server) func([]kyber.Point) Challenge {
	sendCommitsReceiveChallenge := func(pKClientCommitments []kyber.Point) Challenge {
		return signDummyChallenge(cs, servers, pKClientCommitments)
	}
	return sendCommitsReceiveChallenge
}

//// test helper that returns dummy "channel" to act as a dummy server/verifier.
//// this "channel" always replace the sent commitments by random ones before signing the challenge,
//// hack used to simulate a malicious prover (instead of actually building one just for the test case)
//// that doesn't know any secret, and want, after the proof run to replace
//// commitments in proof transcript by commitments built using verification formula
//func newBadCommitsServerChannels(cs kyber.Scalar, servers []Server) func([]kyber.Point) Challenge {
//	sendCommitsReceiveChallenge := func(pKClientCommitments []kyber.Point) Challenge {
//		// TODO FIXME share helper... or ...? (I have a randomPointSlice helper in daga_login.testing => import cycle...)
//		randomPointSlice := make([]kyber.Point, 0, len(pKClientCommitments))
//		for _, _ = range pKClientCommitments {
//			randomPointSlice = append(randomPointSlice, suite.Point().Pick(suite.RandomStream()))
//		}
//		return signDummyChallenge(cs, servers, randomPointSlice)
//	}
//	return sendCommitsReceiveChallenge
//}

// test helper that returns dummy "channel" to act as a dummy server/verifier
// that performs a stupid MITM on originalChannel to return a tampered challenge
// TODO refactor in fact not needed => accept 1 challenge instead of 2 parameters
func newTamperedServerChannels(evilCs kyber.Scalar, evilSigs []ServerSignature, /*replaceSigs bool, originalChannel func([]kyber.Point) Challenge*/) func([]kyber.Point) Challenge {
	sendCommitsReceiveChallenge := func(pKClientCommitments []kyber.Point) Challenge {
		//originalChallenge := originalChannel(pKClientCommitments)
		//sigs :=
		evilChallenge := Challenge{Cs: evilCs, Sigs: evilSigs}
		return evilChallenge
	}
	return sendCommitsReceiveChallenge
}

func TestNewClientProof(t *testing.T) {
	// setup, test context, clients, servers
	clients, servers, context, _ := generateTestContext(suite, rand.Intn(10)+2, rand.Intn(10)+2)

	// setup dummy server "channels"
	cs := suite.Scalar().Pick(suite.RandomStream())
	dummyServerChannel := newDummyServerChannels(cs, servers)
	sendCommitsReceiveChallenge := dummyServerChannel

	// normal execution, create client proof
	_, Y := context.Members()
	tagAndCommitments, s := newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])

	proof, err := newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	require.NoError(t, err, "newClientProof returned an error on valid inputs")
	commits, responses, subChallenges := proof.T, proof.R, proof.C
	// FIXME not sure whether these tests are pertinent or well written... they are testing the proof framework...not my code
	require.Equal(t, len(commits), 3*len(clients))
	require.Equal(t, len(subChallenges), len(clients))
	require.Equal(t, len(responses), 2*len(clients))

	validSigs := proof.Cs.Sigs
	//Incorrect challenges
	var fake kyber.Scalar
	for {
		fake = suite.Scalar().Pick(suite.RandomStream())
		if !fake.Equal(cs) {
			break
		}
	}
	sendCommitsReceiveChallenge = newTamperedServerChannels(fake, validSigs, /*dummyServerChannel*/)
	proof, err = newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	commits, responses, subChallenges = proof.T, proof.R, proof.C
	require.Error(t, err, "newClientProof returned no error on invalid server inputs (altered challenge)")
	require.Equal(t, ClientProof{}, proof, "proof not \"zero\" on error")

	//Signature modification
	newsig := append(validSigs[0].Sig, []byte("A")...)
	newsig = newsig[1:]
	wrongSigs := make([]ServerSignature, len(validSigs))
	copy(wrongSigs, validSigs)
	wrongSigs[0].Sig = newsig
	sendCommitsReceiveChallenge = newTamperedServerChannels(cs, wrongSigs, /*dummyServerChannel*/)

	proof, err = newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	require.Error(t, err, "newClientProof returned no error on invalid server inputs (altered signature)")
	require.Equal(t, ClientProof{}, proof, "proof not \"zero\" on error")
}

// returns a proof transcript that doesn't contains the original proof messages and replace the commitments by ones built
// using the verification formula and the knowledge of the challenge, used to test that nobody is fooled by such an attempt.
// mostly copy pasted from newClientProof
func newMaliciousClientProof(suite Suite, context AuthenticationContext,
	client Client,
	tagAndCommitments initialTagAndCommitments,
	s kyber.Scalar,
	sendCommitsReceiveChallenge func([]kyber.Point) Challenge) (ClientProof, error) {

	if context == nil {
		return ClientProof{}, errors.New("nil context")
	}
	X, Y := context.Members()

	if len(X) <= 1 {
		return ClientProof{}, errors.New("newMaliciousClientProof: there is only one client in the context, this means DAGA is pointless")
		// moreover the following code (and more or less DAGA paper) assumes that there is at least 2 clients/predicates
		// in the context/OR-predicate, if this condition is not met there won't be an "subChallenges" to generate by the
		// prover => he won't send them by calling Put, but we wait for them !!
		// in case this assumption needs to be relaxed, a test should be added to the proverContext.receiveChallenges() method
	}

	//construct the proof.Prover for client's PK predicate and its proof.ProverContext
	prover := newClientProver(suite, context, tagAndCommitments, client, s)
	proverCtx := newClientProverCtx(suite, len(X))

	//3-move interaction with server
	//	start the proof.Prover and proof machinery in new goroutine  // TODO maybe create a function
	var P ClientProof
	var proverErr error
	go func() {
		defer close(proverCtx.responsesChan)
		proverErr = prover(proverCtx)
	}()

	//	get initial commitments from running Prover (and discard them)
	if _, err := proverCtx.commitments(); err != nil {
		return ClientProof{}, errors.New("newMaliciousClientProof:" + err.Error())
	}

	//	forward random commitments to random remote server/verifier
	//	and receive master challenge from remote server(s)
	randomPointSlice := make([]kyber.Point, 0, 3*len(X))
	for i:=0; i<3*len(X); i++ {
		randomPointSlice = append(randomPointSlice, suite.Point().Pick(suite.RandomStream()))
	}
	challenge := sendCommitsReceiveChallenge(randomPointSlice)

	if err := challenge.VerifySignatures(suite, Y, randomPointSlice); err != nil {
		return ClientProof{}, errors.New("newMaliciousClientProof:" + err.Error())
	}
	P.Cs = challenge

	//	forward master challenge to running Prover in order to continue the proof process, and receive the sub-challenges from Prover
	P.C = proverCtx.receiveChallenges(P.Cs.Cs)

	//	get final responses from Prover
	if responses, err := proverCtx.responses(); err != nil {
		// TODO onet.log something
		return ClientProof{}, errors.New("newMaliciousClientProof:" + err.Error())
	} else {
		P.R = responses
	}

	//check return value of the now done proof.Prover
	if proverErr != nil { // here no race, we are sure that Prover is done since responses() returns only after response chan is closed
		// TODO onet.log something
		return ClientProof{}, errors.New("newMaliciousClientProof:" + proverErr.Error())
	}

	// build new malicious commitments (using verification formula) to include in transcript
	T := make([]kyber.Point, 3*len(X))
	for i:=0; i<len(X); i++ {
		// satisfy the kyber.proof framework "contract" see issue/comments/fixme added in kyber.proof
		r0, r1 := P.R[2*i], P.R[2*i+1]
		if i != client.Index() {  // swap, they were sent in that non-obvious order because of internal details of kyber.proof framework => would deserve a reordering layer or at least some documentation IMO
			r0, r1 = P.R[2*i+1], P.R[2*i]
		}
		T[3*i] = suite.Point().Add(suite.Point().Mul(P.C[i], X[i]), suite.Point().Mul(r0, nil))
		//fmt.Printf("c: %v\nr: %v\n", proof.C[i], r0)
		//fmt.Printf(" => t = cX%d + rG = %v\n\n", i, T[3*i])
		T[3*i + 1] = suite.Point().Add(suite.Point().Mul(P.C[i], tagAndCommitments.SCommits[len(tagAndCommitments.SCommits)-1]), suite.Point().Mul(r1, nil))
		//fmt.Printf("c: %v\nr: %v\n", proof.C[i], r1)
		//fmt.Printf("t = cSm + rG = %v\n\n", T[3*i+1])
		T[3*i + 2] = suite.Point().Add(suite.Point().Mul(P.C[i], tagAndCommitments.T0), suite.Point().Mul(r1, context.ClientsGenerators()[i]))
		//fmt.Printf("c: %v\nr: %v\n", proof.C[i], r1)
		//fmt.Printf("t = cT0 + rH%d = %v\n\n\n\n", i, T[3*i+2])
	}
	P.T = T
	return P, nil
}

func TestVerifyClientProof(t *testing.T) {
	// setup, test context, clients, servers
	clients, servers, context, _ := generateTestContext(suite, rand.Intn(10)+2, rand.Intn(10)+2)

	// setup dummy server "channels"
	cs := suite.Scalar().Pick(suite.RandomStream())
	sendCommitsReceiveChallenge := newDummyServerChannels(cs, servers)

	proverIndex := 0

	// create valid proof and auth. message
	_, Y := context.Members()
	tagAndCommitments, s := newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[proverIndex].Index()])
	proof, _ := newClientProof(suite, context, clients[proverIndex], *tagAndCommitments, s, sendCommitsReceiveChallenge)

	clientMsg := AuthenticationMessage{
		C:                        context,
		initialTagAndCommitments: *tagAndCommitments,
		P0:                       proof,
	}

	//Normal execution
	require.NoError(t, validateAuthenticationMessage(suite, clientMsg), "Cannot validate valid client message")
	require.NoError(t, verifyAuthenticationMessage(suite, clientMsg), "Cannot verify valid client proof")

	//Modify the value of some commitments
	scratchMsg := clientMsg
	i := rand.Intn(len(clients))
	ttemp := scratchMsg.P0.T[3*i].Clone()
	scratchMsg.P0.T[3*i] = suite.Point().Null()
	require.Error(t, verifyAuthenticationMessage(suite, scratchMsg), "Incorrect check of t at index %d", 3*i)

	scratchMsg.P0.T[3*i] = ttemp.Clone()
	ttemp = scratchMsg.P0.T[3*i+1].Clone()
	scratchMsg.P0.T[3*i+1] = suite.Point().Null()
	require.Error(t, verifyAuthenticationMessage(suite, scratchMsg), "Incorrect check of t at index %d", 3*i+1)

	scratchMsg.P0.T[3*i+1] = ttemp.Clone()
	ttemp = scratchMsg.P0.T[3*i+2].Clone()
	scratchMsg.P0.T[3*i+2] = suite.Point().Null()
	require.Error(t, verifyAuthenticationMessage(suite, scratchMsg), "Incorrect check of t at index %d", 3*i+2)

	scratchMsg.P0.T[3*i+2] = ttemp.Clone()

	//tamper the challenge
	scratchMsg.P0.Cs.Cs = suite.Scalar().Zero()
	require.Error(t, verifyAuthenticationMessage(suite, scratchMsg), "Incorrect check of the challenge")
	scratchMsg.P0.Cs = clientMsg.P0.Cs

	// run the PKClient proof with wrong commitments then set the commitments using the verification formula and the now known challenge (cheat, alter transcript after protocol run..)
	badProof, _ := newMaliciousClientProof(suite, context, clients[proverIndex], *tagAndCommitments, s, newDummyServerChannels(cs, servers))

	clientMsg.P0 = badProof
	require.EqualError(t, verifyAuthenticationMessage(suite, clientMsg), "verifyAuthenticationMessage:verifyClientProof: proof transcript not accepted, commitments or challenge mismatch",
		"Incorrect check of the commitments, malicious commitments built using verifier formula accepted")
}

func TestGetFinalLinkageTag(t *testing.T) {
	// setup, test context, clients, servers, and "network channel"
	clients, servers, context, _ := generateTestContext(suite, rand.Intn(10)+2, rand.Intn(10)+1)

	// setup dummy server "channels"
	cs := suite.Scalar().Pick(suite.RandomStream())
	sendCommitsReceiveChallenge := newDummyServerChannels(cs, servers)

	//Create test authMsg M0 // TODO instead of these (above and below tests too) use NewAuthMessage (=> make new Auth message easily testable by adding server channels parameters)
	_, Y := context.Members()
	tagAndCommitments, s := newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])

	proof, _ := newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	clientMessage := AuthenticationMessage{
		C:                        context,
		initialTagAndCommitments: *tagAndCommitments,
		P0:                       proof,
	}

	//Create the initial server message
	servMsg := ServerMessage{Request: clientMessage, Proofs: nil, Tags: nil, Sigs: nil, Indexes: nil}

	//Run ServerProtocol on each server
	for i := range servers {
		err := ServerProtocol(suite, &servMsg, servers[i])
		require.NoError(t, err, "server %v returned an error while processing valid auth. request", i)
	}

	//Normal execution for a normal client
	Tf, err := GetFinalLinkageTag(suite, context, servMsg)
	require.NoError(t, err, "Cannot extract final linkage tag")
	require.NotNil(t, Tf, "Cannot extract final linkage tag")

	//Empty inputs
	Tf, err = GetFinalLinkageTag(suite, nil, servMsg)
	require.Error(t, err, "wrong check: Empty context")
	require.Nil(t, Tf, "wrong check: Empty context")

	Tf, err = GetFinalLinkageTag(suite, context, ServerMessage{})
	require.Error(t, err, "wrong check: Empty context")
	require.Nil(t, Tf, "wrong check: Empty context")

	//Change a signature
	servMsg.Sigs[0].Sig = append(servMsg.Sigs[0].Sig[1:], servMsg.Sigs[0].Sig[0])
	Tf, err = GetFinalLinkageTag(suite, context, servMsg)
	require.Error(t, err, "Invalid signature accepted")
	require.Nil(t, Tf, "Invalid signature accepted")

	//Revert the change
	servMsg.Sigs[0].Sig = append([]byte{0x0}, servMsg.Sigs[0].Sig...)
	servMsg.Sigs[0].Sig[0] = servMsg.Sigs[0].Sig[len(servMsg.Sigs[0].Sig)-1]
	servMsg.Sigs[0].Sig = servMsg.Sigs[0].Sig[:len(servMsg.Sigs[0].Sig)-2]

	//Misbehaving clients
	// TODO add mutliple different scenarios
	clients, servers, context, _ = generateTestContext(suite, rand.Intn(10)+2, 1)
	_, Y = context.Members()
	tagAndCommitments, s = newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])
	// 1 server, bad tagAndCommitments, invalid proof => reject proof => cannot get (even try to get) final tag
	S := tagAndCommitments.SCommits

	S[2] = suite.Point().Null()
	sendCommitsReceiveChallenge = newDummyServerChannels(cs, servers)
	proof, err = newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	clientMessage = AuthenticationMessage{
		C:                        context,
		initialTagAndCommitments: *tagAndCommitments,
		P0:                       proof,
	}

	//Create the initial server message
	servMsg = ServerMessage{
		Request: clientMessage,
		Proofs:  nil,
		Tags:    nil,
		Sigs:    nil,
		Indexes: nil,
	}

	//Run ServerProtocol on each server
	for i := range servers {
		err := ServerProtocol(suite, &servMsg, servers[i])
		require.Error(t, err, "server %v returned no error while processing invalid auth. request", i)
	}
	Tf, err = GetFinalLinkageTag(suite, context, servMsg)
	require.Error(t, err, "can extract final linkage tag for an invalid request, should have returned an error")
	require.Nil(t, Tf, "Tf not nil on error")
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// 1 server, bad tagAndCommitments, valid proof => flag as misbehaving => receive null final tag
	//Assemble the client message
	S = tagAndCommitments.SCommits
	S[2] = suite.Point().Null()
	tagAndCommitments.T0.Set(suite.Point().Null())
	sendCommitsReceiveChallenge = newDummyServerChannels(cs, servers)
	proof, err = newClientProof(suite, context, clients[0], *tagAndCommitments, suite.Scalar().Zero(), sendCommitsReceiveChallenge)
	clientMessage = AuthenticationMessage{
		C:                        context,
		initialTagAndCommitments: *tagAndCommitments,
		P0:                       proof,
	}

	//Create the initial server message
	servMsg = ServerMessage{
		Request: clientMessage,
		Proofs:  nil,
		Tags:    nil,
		Sigs:    nil,
		Indexes: nil,
	}

	//Run ServerProtocol on each server
	for i := range servers {
		err := ServerProtocol(suite, &servMsg, servers[i])
		require.NoError(t, err, "server %v returned an error while processing auth. request of a misbehaving client", i)
	}
	Tf, err = GetFinalLinkageTag(suite, context, servMsg)
	require.NoError(t, err, "cannot extract final linkage tag for a misbehaving client")
	require.True(t, Tf.Equal(suite.Point().Null()), "Tf not Null for a misbehaving client")

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// n>1 servers, bad tagAndCommitments, valid proof => flag as misbehaving => receive null final tag
	clients, servers, context, _ = generateTestContext(suite, rand.Intn(10)+2, rand.Intn(10)+2)
	//Assemble the client message
	_, Y = context.Members()
	tagAndCommitments, s = newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])
	S = tagAndCommitments.SCommits
	S[2] = suite.Point().Null()
	sendCommitsReceiveChallenge = newDummyServerChannels(cs, servers)
	proof, err = newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	clientMessage = AuthenticationMessage{
		C:                        context,
		initialTagAndCommitments: *tagAndCommitments,
		P0:                       proof,
	}

	//Create the initial server message
	servMsg = ServerMessage{
		Request: clientMessage,
		Proofs:  nil,
		Tags:    nil,
		Sigs:    nil,
		Indexes: nil,
	}

	//Run ServerProtocol on each server
	for i := range servers {
		err := ServerProtocol(suite, &servMsg, servers[i])
		require.NoError(t, err, "server %v returned an error while processing auth. request of a misbehaving client", i)
	}
	Tf, err = GetFinalLinkageTag(suite, context, servMsg)
	require.NoError(t, err, "cannot extract final linkage tag for a misbehaving client")
	require.True(t, Tf.Equal(suite.Point().Null()), "Tf not Null for a misbehaving client")
}

// TODO merge or rearrange with some tests above as lots of things are redundant...or should belong to same test
// e.g see testverifyclientproof and its tampering of the p0.commitments
// + fundamentaly verify message => verify proof, so either split accordingly and test only message related things reps. proof related things in both
// or merge them together in same test and test everything
// FIXME or (but I won't lose more time on this) rewrite everything to follow best testing practises (more better named small tests for a start)
func TestValidateClientMessage(t *testing.T) {
	// setup, test context, clients, servers, and "network channel"
	clients, servers, context, _ := generateTestContext(suite, rand.Intn(10)+1, rand.Intn(10)+1)

	// setup dummy server "channels"
	cs := suite.Scalar().Pick(suite.RandomStream())
	sendCommitsReceiveChallenge := newDummyServerChannels(cs, servers)

	//Create test authMsg M0
	_, Y := context.Members()
	tagAndCommitments, s := newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])
	proof, _ := newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	clientMessage := AuthenticationMessage{
		C:                        context,
		initialTagAndCommitments: *tagAndCommitments,
		P0:                       proof,
	}

	//Normal execution
	// TODO already tested somewhere above...
	require.NoError(t, verifyAuthenticationMessage(suite, clientMessage), "Cannot verify valid client proof")

	//Modifying the length of various elements
	ScratchMsg := clientMessage
	ScratchMsg.P0.C = append(ScratchMsg.P0.C, suite.Scalar().Pick(suite.RandomStream()))
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for c: %d instead of %d", len(ScratchMsg.P0.C), len(clients))

	ScratchMsg.P0.C = ScratchMsg.P0.C[:len(clients)-1]
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for c: %d instead of %d", len(ScratchMsg.P0.C), len(clients))

	ScratchMsg = clientMessage
	ScratchMsg.P0.R = append(ScratchMsg.P0.R, suite.Scalar().Pick(suite.RandomStream()))
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for r: %d instead of %d", len(ScratchMsg.P0.C), len(clients))

	ScratchMsg.P0.R = ScratchMsg.P0.R[:2*len(clients)-1]
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for r: %d instead of %d", len(ScratchMsg.P0.C), len(clients))

	ScratchMsg = clientMessage
	ScratchMsg.P0.T = append(ScratchMsg.P0.T, suite.Point().Mul(suite.Scalar().Pick(suite.RandomStream()), nil))
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for t: %d instead of %d", len(ScratchMsg.P0.C), len(clients))

	ScratchMsg.P0.T = ScratchMsg.P0.T[:3*len(clients)-1]
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for t: %d instead of %d", len(ScratchMsg.P0.C), len(clients))

	ScratchMsg = clientMessage
	ScratchMsg.SCommits = append(ScratchMsg.SCommits, suite.Point().Mul(suite.Scalar().Pick(suite.RandomStream()), nil))
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for S: %d instead of %d", len(ScratchMsg.SCommits), len(servers)+2)

	ScratchMsg.SCommits = ScratchMsg.SCommits[:len(servers)+1]
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect length check for S: %d instead of %d", len(ScratchMsg.SCommits), len(servers)+2)

	//Modify the value of the generator in S[1]
	ScratchMsg = clientMessage
	ScratchMsg.SCommits[1] = suite.Point().Mul(suite.Scalar().Pick(suite.RandomStream()), nil)
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Incorrect check for the generator in S[1]")

	ScratchMsg.SCommits[1] = suite.Point().Mul(suite.Scalar().One(), nil)

	//Remove T0
	ScratchMsg.T0 = nil
	require.Error(t, verifyAuthenticationMessage(suite, ScratchMsg), "Accepts a empty T0")
}

func TestToBytes_ClientMessage(t *testing.T) {
	// setup, test context, clients, servers, and "network channel"
	clients, servers, context, _ := generateTestContext(suite, rand.Intn(10)+2, rand.Intn(10)+1)

	// setup dummy server "channels"
	cs := suite.Scalar().Pick(suite.RandomStream())
	sendCommitsReceiveChallenge := newDummyServerChannels(cs, servers)

	//Create test authMsg M0  // TODO instead of these (above and below tests too) use NewAuthMessage (=> make new Auth message easily testable by adding server channels parameters)
	_, Y := context.Members()
	tagAndCommitments, s := newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])
	proof, _ := newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)
	clientMessage := AuthenticationMessage{
		C:                        context,
		initialTagAndCommitments: *tagAndCommitments,
		P0:                       proof,
	}

	//Normal execution
	data, err := clientMessage.ToBytes()
	require.NoError(t, err, "Cannot convert valid Client Message to bytes")
	require.NotNil(t, data, "Data is empty for a correct Client Message")
}

func TestToBytes_ClientProof(t *testing.T) {
	// setup, test context, clients, servers, and "network channel"
	clients, servers, context, _ := generateTestContext(suite, rand.Intn(10)+2, rand.Intn(10)+1)

	// setup dummy server "channels"
	cs := suite.Scalar().Pick(suite.RandomStream())
	sendCommitsReceiveChallenge := newDummyServerChannels(cs, servers)

	//Create test client proof
	_, Y := context.Members()
	tagAndCommitments, s := newInitialTagAndCommitments(suite, Y, context.ClientsGenerators()[clients[0].Index()])
	proof, _ := newClientProof(suite, context, clients[0], *tagAndCommitments, s, sendCommitsReceiveChallenge)

	//Normal execution
	data, err := proof.ToBytes()
	require.NoError(t, err, "Cannot convert valid proof to bytes")
	require.NotNil(t, data, "Data is empty for a correct proof")
}
