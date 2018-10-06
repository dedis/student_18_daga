package daga

import (
	"fmt"
	"github.com/dedis/kyber"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

// FIXME review/see if the test are sound

func TestNewClient(t *testing.T) {
	//Normal execution
	i := rand.Int()
	s := suite.Scalar().Pick(suite.RandomStream())
	client, err := NewClient(i, s)
	if err != nil || client.index != i || !client.key.Private.Equal(s) {
		t.Error("Cannot initialize a new client with a given private key")
	}

	client, err = NewClient(i, nil)
	if err != nil {
		t.Error("Cannot create a new client without a private key")
	}

	//Invalid input
	client, err = NewClient(-2, s)
	if err == nil {
		t.Error("Wrong check: Invalid index")
	}
}

func TestCreateRequest(t *testing.T) {
	//Normal execution
	clients, servers, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
	tagAndCommitments, s, err := newInitialTagAndCommitments(context.g.y, context.h[clients[0].index])
	T0, S := tagAndCommitments.t0, tagAndCommitments.sCommits
	if err != nil {
		assert.Equal(t, T0, nil, "T0 not nil on error")
		assert.Equal(t, S, nil, "S not nil on error")
		assert.Equal(t, s, nil, "s not nil on error")
		t.Error("Cannot create request under regular context")
	}

	if T0 == nil {
		t.Error("T0 empty")
	}
	if T0.Equal(suite.Point().Null()) {
		t.Error("T0 is the null point")
	}

	if S == nil {
		t.Error("S is empty")
	}
	if len(S) != len(servers)+2 {
		t.Errorf("S has the wrong length: %d instead of %d", len(S), len(servers)+2)
	}
	for i, temp := range S {
		if temp.Equal(suite.Point().Null()) {
			t.Errorf("Null point in S at position %d", i)
		}
	}

	if s == nil {
		t.Error("s is empty")
	}
}

// GenerateProofCommitments creates and returns the client's commitments t
// TODO old implementation used for regression testing of the current implementation making use of the kyber.proof framework
func generateProofCommitments(clientIndex int, context *authenticationContext, T0 kyber.Point, s kyber.Scalar) ([]kyber.Point, []kyber.Scalar, []kyber.Scalar) {
	//Generates w randomly except for w[client.index] = 0
	w := make([]kyber.Scalar, len(context.h))
	for i := range w {
		w[i] = suite.Scalar().SetInt64(42)//Pick(suite.RandomStream())
	}
	w[clientIndex] = suite.Scalar().Zero()

	//Generates random v (2 per client)
	v := make([]kyber.Scalar, 2*len(context.h))
	for i := range v {
		v[i] = suite.Scalar().SetInt64(42)//Pick(suite.RandomStream())
	}

	//Generates the commitments t (3 per clients)
	t := make([]kyber.Point, 3*len(context.h))
	for i := 0; i < len(context.h); i++ {
		a := suite.Point().Mul(w[i], context.g.x[i])
		b := suite.Point().Mul(v[2*i], nil)
		t[3*i] = suite.Point().Add(a, b)

		Sm := suite.Point().Mul(s, nil)
		c := suite.Point().Mul(w[i], Sm)
		d := suite.Point().Mul(v[2*i+1], nil)
		t[3*i+1] = suite.Point().Add(c, d)

		e := suite.Point().Mul(w[i], T0)
		f := suite.Point().Mul(v[2*i+1], context.h[i])
		t[3*i+2] = suite.Point().Add(e, f)
	}

	return t, v, w
}

func TestGenerateProofCommitmentsAndResponses(t *testing.T) {
	clients, _, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
	tagAndCommitments, s, _ := newInitialTagAndCommitments(context.g.y, context.h[clients[0].index])
	T0, _ := tagAndCommitments.t0, tagAndCommitments.sCommits

	// QUESTION FIXME find way to give same random coins to both implementations to compare results, didn't manage to make seed() work....
	refCommits,_,_ := generateProofCommitments(0, context, T0, s)

	// dummy channel to receive the commitments (they will be part of the returned proof)
	// and dummy channel to send a dummy challenge as we are only interested in the commitments
	// "push"/"pull" from the perspective of newClientProof()
	pushCommitments := make(chan []kyber.Point)
	pullChallenge := make(chan kyber.Scalar)
	go func() {
		<- pushCommitments
		// TODO sign challenge
		pullChallenge <- suite.Scalar().Pick(suite.RandomStream())
	}()

	proof, _ := newClientProof(*context, clients[0], *tagAndCommitments, s, pushCommitments, pullChallenge)
	commits := proof.t

	if commits == nil {
		t.Error("t is empty")
	}

	if len(commits) != 3*len(clients) {//|| len(commits) != len(refCommits) {
		t.Errorf("Wrong length of t: %d instead of %d", len(commits), 3*len(clients))
	}

	ok := func() bool {
		for i,ref := range refCommits {
			if !ref.Equal(commits[i]) {
				return false
			}
		}
		return true
	}

	fmt.Println(refCommits, "\n#######################################")
	fmt.Println(commits)

	if !ok() {
		t.Error("regression, commitments differ from previous manual implementation")
	}
}


// TODO clean and merge with previous test + add new tests for sign/verify when done moving to EdDSA
//func TestGenerateProofResponses(t *testing.T) {
//	clients, servers, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
//	tagAndCommitments, s, _ := newInitialTagAndCommitments(context.g.y, context.h[clients[0].index])
//	T0, _ := tagAndCommitments.t0, tagAndCommitments.sCommits
//	_, v, w := generateProofCommitments(0, context, T0, s)
//
//	//Dumb challenge generation
//	cs := suite.Scalar().Pick(suite.RandomStream())
//	msg, _ := cs.MarshalBinary()
//	var sigs []serverSignature
//	//Make each test server sign the challenge
//	for _, server := range servers {
//		sig, e := ECDSASign(server.private, msg)
//		if e != nil {
//			t.Errorf("Cannot sign the challenge for server %d", server.index)
//		}
//		sigs = append(sigs, serverSignature{index: server.index, sig: sig})
//	}
//	challenge := Challenge{cs: cs, sigs: sigs}
//
//	//Normal execution
//	c, r, err := clients[0].GenerateProofResponses(context, s, &challenge, v, w)
//	if err != nil {
//		t.Error("Cannot generate proof responses")
//	}
//
//	if len(c) != len(clients) {
//		t.Errorf("Wrong length of c: %d instead of %d", len(c), len(clients))
//	}
//	if len(r) != 2*len(clients) {
//		t.Errorf("Wrong length of r: %d instead of %d", len(r), 2*len(clients))
//	}
//
//	for i, temp := range c {
//		if temp == nil {
//			t.Errorf("nil in c at index %d", i)
//		}
//	}
//	for i, temp := range r {
//		if temp == nil {
//			t.Errorf("nil in r at index %d", i)
//		}
//	}
//
//	//Incorrect challenges
//	var fake kyber.Scalar
//	for {
//		fake = suite.Scalar().Pick(suite.RandomStream())
//		if !fake.Equal(cs) {
//			break
//		}
//	}
//	wrongChallenge := Challenge{cs: fake, sigs: sigs}
//	c, r, err = clients[0].GenerateProofResponses(context, s, &wrongChallenge, v, w)
//	if err == nil {
//		t.Error("Cannot verify the message")
//	}
//	if c != nil {
//		t.Error("c not nil on message error")
//	}
//	if r != nil {
//		t.Error("r not nil on message error")
//	}
//
//	//Signature modification
//	newsig := append([]byte("A"), sigs[0].sig...)
//	newsig = newsig[:len(sigs[0].sig)]
//	sigs[0].sig = newsig
//	SigChallenge := Challenge{cs: cs, sigs: sigs}
//	c, r, err = clients[0].GenerateProofResponses(context, s, &SigChallenge, v, w)
//	if err == nil {
//		t.Error("Cannot verify the message")
//	}
//	if c != nil {
//		t.Error("c not nil on signature error")
//	}
//	if r != nil {
//		t.Error("r not nil on signature error")
//	}
//}

func TestVerifyClientProof(t *testing.T) {
	clients, _, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
	tagAndCommitments, s, _ := newInitialTagAndCommitments(context.g.y, context.h[clients[0].index])

	pushCommitments := make(chan []kyber.Point)
	pullChallenge := make(chan kyber.Scalar)
	go func() {
		<- pushCommitments
		// TODO sign challenge
		//Dumb challenge generation
		pullChallenge <- suite.Scalar().Pick(suite.RandomStream())
	}()
	proof, err := newClientProof(*context, clients[0], *tagAndCommitments, s, pushCommitments, pullChallenge)
	if err != nil {
		t.Error("newClientProof returned an error:", err)
	}

	clientMsg := authenticationMessage{
		c: *context,
		initialTagAndCommitments: *tagAndCommitments,
		p0:  proof,
	}

	//Normal execution
	check := verifyClientProof(clientMsg)
	if !check {
		t.Error("Cannot verify client proof")
	}

	//Modify the value of some commitments
	ScratchMsg := clientMsg
	i := rand.Intn(len(clients))
	ttemp := ScratchMsg.p0.t[3*i].Clone()
	ScratchMsg.p0.t[3*i] = suite.Point().Null()
	check = verifyClientProof(ScratchMsg)
	if check {
		t.Errorf("Incorrect check of t at index %d", 3*i)
	}
	ScratchMsg.p0.t[3*i] = ttemp.Clone()

	ttemp = ScratchMsg.p0.t[3*i+1].Clone()
	ScratchMsg.p0.t[3*i+1] = suite.Point().Null()
	check = verifyClientProof(ScratchMsg)
	if check {
		t.Errorf("Incorrect check of t at index %d", 3*i+1)
	}
	ScratchMsg.p0.t[3*i+1] = ttemp.Clone()

	ttemp = ScratchMsg.p0.t[3*i+2].Clone()
	ScratchMsg.p0.t[3*i+2] = suite.Point().Null()
	check = verifyClientProof(ScratchMsg)
	if check {
		t.Errorf("Incorrect check of t at index %d", 3*i+2)
	}
	ScratchMsg.p0.t[3*i+2] = ttemp.Clone()

	//Modify the value of the challenge
	ScratchMsg.p0.cs = suite.Scalar().Zero()
	check = verifyClientProof(ScratchMsg)
	if check {
		t.Errorf("Incorrect check of the challenge")
	}
}


// TODO will become kind of a TestnewAuthenticationMessage
//func TestAssembleMessage(t *testing.T) {
//	clients, servers, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
//	tagAndCommitments, s, _ := newInitialTagAndCommitments(context.g.y, context.h[clients[0].index])
//	T0, S := tagAndCommitments.t0, tagAndCommitments.sCommits
//	tclient, v, w := clients[0].GenerateProofCommitments(context, T0, s)
//
//	//Dumb challenge generation
//	cs := suite.Scalar().Pick(suite.RandomStream())
//	msg, _ := cs.MarshalBinary()
//	var sigs []serverSignature
//	//Make each test server sign the challenge
//	for _, server := range servers {
//		sig, e := ECDSASign(server.private, msg)
//		if e != nil {
//			t.Errorf("Cannot sign the challenge for server %d", server.index)
//		}
//		sigs = append(sigs, serverSignature{index: server.index, sig: sig})
//	}
//	challenge := Challenge{cs: cs, sigs: sigs}
//
//	c, r, _ := clients[0].GenerateProofResponses(context, s, &challenge, v, w)
//
//	//Normal execution
//	clientMsg := clients[0].AssembleMessage(context, &S, T0, &challenge, tclient, c, r)
//	if !ValidateClientMessage(clientMsg) || clientMsg == nil {
//		t.Error("Cannot assemble a client message")
//	}
//
//	//Empty inputs
//	clientMsg = clients[0].AssembleMessage(nil, &S, T0, &challenge, tclient, c, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty context")
//	}
//	clientMsg = clients[0].AssembleMessage(context, nil, T0, &challenge, tclient, c, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty S")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &[]kyber.Point{}, T0, &challenge, tclient, c, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: len(S) = 0")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, nil, &challenge, tclient, c, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty T0")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, T0, nil, tclient, c, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty challenge")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, T0, &challenge, nil, c, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty t")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, T0, &challenge, &[]kyber.Point{}, c, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: len(t) = 0 ")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, T0, &challenge, tclient, nil, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty c")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, T0, &challenge, tclient, &[]kyber.Scalar{}, r)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty ")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, T0, &challenge, tclient, c, nil)
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty r")
//	}
//	clientMsg = clients[0].AssembleMessage(context, &S, T0, &challenge, tclient, c, &[]kyber.Scalar{})
//	if clientMsg != nil {
//		t.Error("Wrong check: Empty ")
//	}
//
//}

//func TestGetFinalLinkageTag(t *testing.T) {
//	clients, servers, context, _ := generateTestContext(1, 2)
//	for _, server := range servers {
//		if server.r == nil {
//			t.Errorf("Error in r for server %d", server.index)
//		}
//	}
//	T0, S, s, _ := clients[0].CreateRequest(context)
//	tclient, v, w := clients[0].GenerateProofCommitments(context, T0, s)
//
//	//Dumb challenge generation
//	cs := suite.Scalar().Pick(suite.RandomStream())
//	msg, _ := cs.MarshalBinary()
//	var sigs []serverSignature
//	//Make each test server sign the challenge
//	for _, server := range servers {
//		sig, e := ECDSASign(server.private, msg)
//		if e != nil {
//			t.Errorf("Cannot sign the challenge for server %d", server.index)
//		}
//		sigs = append(sigs, serverSignature{index: server.index, sig: sig})
//	}
//	challenge := Challenge{cs: cs, sigs: sigs}
//
//	c, r, _ := clients[0].GenerateProofResponses(context, s, &challenge, v, w)
//
//	//Assemble the client message
//	clientMessage := ClientMessage{sArray: S, t0: T0, context: *context,
//		proof: ClientProof{cs: cs, c: *c, t: *tclient, r: *r}}
//
//	//Create the initial server message
//	servMsg := ServerMessage{request: clientMessage, proofs: nil, tags: nil, sigs: nil, indexes: nil}
//
//	//Run ServerProtocol on each server
//	for i := range servers {
//		servers[i].ServerProtocol(context, &servMsg)
//	}
//
//	//Normal execution for a normal client
//	Tf, err := clients[0].GetFinalLinkageTag(context, &servMsg)
//	if err != nil || Tf == nil {
//		t.Errorf("Cannot extract final linkage tag:\n%s", err)
//	}
//
//	//Empty inputs
//	Tf, err = clients[0].GetFinalLinkageTag(nil, &servMsg)
//	if err == nil || Tf != nil {
//		t.Errorf("Wrong check: Empty context")
//	}
//	Tf, err = clients[0].GetFinalLinkageTag(context, nil)
//	if err == nil || Tf != nil {
//		t.Errorf("Wrong check: Empty message")
//	}
//
//	//Change a signature
//	servMsg.sigs[0].sig = append(servMsg.sigs[0].sig[1:], servMsg.sigs[0].sig[0])
//	Tf, err = clients[0].GetFinalLinkageTag(context, &servMsg)
//	if err == nil || Tf != nil {
//		t.Errorf("Invalid signature accepted")
//	}
//	//Revert the change
//	servMsg.sigs[0].sig = append([]byte{0x0}, servMsg.sigs[0].sig...)
//	servMsg.sigs[0].sig[0] = servMsg.sigs[0].sig[len(servMsg.sigs[0].sig)-1]
//	servMsg.sigs[0].sig = servMsg.sigs[0].sig[:len(servMsg.sigs[0].sig)-2]
//
//	//Normal execution for a misbehaving client
//	//Assemble the client message
//	S[2] = suite.Point().Null()
//	clientMessage = ClientMessage{sArray: S, t0: T0, context: *context,
//		proof: ClientProof{cs: cs, c: *c, t: *tclient, r: *r}}
//
//	//Create the initial server message
//	servMsg = ServerMessage{request: clientMessage, proofs: nil, tags: nil, sigs: nil, indexes: nil}
//
//	//Run ServerProtocol on each server
//	for i := range servers {
//		servers[i].ServerProtocol(context, &servMsg)
//	}
//	Tf, err = clients[0].GetFinalLinkageTag(context, &servMsg)
//	if err != nil {
//		t.Errorf("Cannot extract final linkage tag for a misbehaving client")
//	}
//	if !Tf.Equal(suite.Point().Null()) {
//		t.Error("Tf not Null for a misbehaving client")
//	}
//}
//
//func TestValidateClientMessage(t *testing.T) {
//	clients, servers, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
//	T0, S, s, _ := clients[0].CreateRequest(context)
//	tproof, v, w := clients[0].GenerateProofCommitments(context, T0, s)
//
//	//Dumb challenge generation
//	cs := suite.Scalar().Pick(suite.RandomStream())
//	msg, _ := cs.MarshalBinary()
//	var sigs []serverSignature
//	//Make each test server sign the challenge
//	for _, server := range servers {
//		sig, e := ECDSASign(server.private, msg)
//		if e != nil {
//			t.Errorf("Cannot sign the challenge for server %d", server.index)
//		}
//		sigs = append(sigs, serverSignature{index: server.index, sig: sig})
//	}
//	challenge := Challenge{cs: cs, sigs: sigs}
//
//	//Generate the final proof
//	c, r, _ := clients[0].GenerateProofResponses(context, s, &challenge, v, w)
//
//	ClientMsg := ClientMessage{context: ContextEd25519{G: Members{X: context.G.X, Y: context.G.Y}, R: context.R, H: context.H},
//		t0:     T0,
//		sArray: S,
//		proof:  ClientProof{c: *c, cs: cs, r: *r, t: *tproof}}
//
//	//Normal execution
//	check := verifyClientProof(ClientMsg)
//	if !check {
//		t.Error("Cannot verify client proof")
//	}
//
//	//Modifying the length of various elements
//	ScratchMsg := ClientMsg
//	ScratchMsg.p0.c = append(ScratchMsg.p0.c, suite.Scalar().Pick(suite.RandomStream()))
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for c: %d instead of %d", len(ScratchMsg.p0.c), len(clients))
//	}
//	ScratchMsg.p0.c = ScratchMsg.p0.c[:len(clients)-1]
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for c: %d instead of %d", len(ScratchMsg.p0.c), len(clients))
//	}
//
//	ScratchMsg = ClientMsg
//	ScratchMsg.p0.r = append(ScratchMsg.p0.r, suite.Scalar().Pick(suite.RandomStream()))
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for r: %d instead of %d", len(ScratchMsg.p0.c), len(clients))
//	}
//	ScratchMsg.p0.r = ScratchMsg.p0.r[:2*len(clients)-1]
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for r: %d instead of %d", len(ScratchMsg.p0.c), len(clients))
//	}
//
//	ScratchMsg = ClientMsg
//	ScratchMsg.p0.t = append(ScratchMsg.p0.t, suite.Point().Mul(suite.Scalar().Pick(suite.RandomStream()), nil))
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for t: %d instead of %d", len(ScratchMsg.p0.c), len(clients))
//	}
//	ScratchMsg.p0.t = ScratchMsg.p0.t[:3*len(clients)-1]
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for t: %d instead of %d", len(ScratchMsg.p0.c), len(clients))
//	}
//
//	ScratchMsg = ClientMsg
//	ScratchMsg.sArray = append(ScratchMsg.sArray, suite.Point().Mul(suite.Scalar().Pick(suite.RandomStream()), nil))
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for S: %d instead of %d", len(ScratchMsg.sArray), len(servers)+2)
//	}
//	ScratchMsg.sArray = ScratchMsg.sArray[:len(servers)+1]
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect length check for S: %d instead of %d", len(ScratchMsg.sArray), len(servers)+2)
//	}
//
//	//Modify the value of the generator in S[1]
//	ScratchMsg = ClientMsg
//	ScratchMsg.sArray[1] = suite.Point().Mul(suite.Scalar().Pick(suite.RandomStream()), nil)
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Incorrect check for the generator in S[1]")
//	}
//	ScratchMsg.sArray[1] = suite.Point().Mul(suite.Scalar().One(), nil)
//
//	//Remove T0
//	ScratchMsg.t0 = nil
//	check = verifyClientProof(ScratchMsg)
//	if check {
//		t.Errorf("Accepts a empty T0")
//	}
//}
//
//func TestToBytes_ClientMessage(t *testing.T) {
//	clients, servers, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
//	T0, S, s, _ := clients[0].CreateRequest(context)
//	tproof, v, w := clients[0].GenerateProofCommitments(context, T0, s)
//
//	//Dumb challenge generation
//	cs := suite.Scalar().Pick(suite.RandomStream())
//	msg, _ := cs.MarshalBinary()
//	var sigs []serverSignature
//	//Make each test server sign the challenge
//	for _, server := range servers {
//		sig, e := ECDSASign(server.private, msg)
//		if e != nil {
//			t.Errorf("Cannot sign the challenge for server %d", server.index)
//		}
//		sigs = append(sigs, serverSignature{index: server.index, sig: sig})
//	}
//	challenge := Challenge{cs: cs, sigs: sigs}
//
//	//Generate the final proof
//	c, r, _ := clients[0].GenerateProofResponses(context, s, &challenge, v, w)
//
//	ClientMsg := ClientMessage{context: ContextEd25519{G: Members{X: context.G.X, Y: context.G.Y}, R: context.R, H: context.H},
//		t0:     T0,
//		sArray: S,
//		proof:  ClientProof{c: *c, cs: cs, r: *r, t: *tproof}}
//
//	//Normal execution
//	data, err := ClientMsg.ToBytes()
//	if err != nil {
//		t.Error("Cannot convert valid Client Message to bytes")
//	}
//	if data == nil {
//		t.Error("Data is empty for a correct Client Message")
//	}
//}
//
//func TestToBytes_ClientProof(t *testing.T) {
//	clients, servers, context, _ := generateTestContext(rand.Intn(10)+1, rand.Intn(10)+1)
//	T0, _, s, _ := clients[0].CreateRequest(context)
//	tproof, v, w := clients[0].GenerateProofCommitments(context, T0, s)
//
//	//Dumb challenge generation
//	cs := suite.Scalar().Pick(suite.RandomStream())
//	msg, _ := cs.MarshalBinary()
//	var sigs []serverSignature
//	//Make each test server sign the challenge
//	for _, server := range servers {
//		sig, e := ECDSASign(server.private, msg)
//		if e != nil {
//			t.Errorf("Cannot sign the challenge for server %d", server.index)
//		}
//		sigs = append(sigs, serverSignature{index: server.index, sig: sig})
//	}
//	challenge := Challenge{cs: cs, sigs: sigs}
//
//	//Generate the final proof
//	c, r, _ := clients[0].GenerateProofResponses(context, s, &challenge, v, w)
//
//	proof := clientProof{c: *c, cs: cs, r: *r, t: *tproof}
//
//	//Normal execution
//	data, err := proof.ToBytes()
//	if err != nil {
//		t.Error("Cannot convert valid proof to bytes")
//	}
//	if data == nil {
//		t.Error("Data is empty for a correct proof")
//	}
//}
