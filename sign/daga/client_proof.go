package daga

// This files contains everything related to the client's HVZK proof of knowledge PKClient used by daga to authenticate a client.
// Notably the code used to build the OR-predicate and generate a prover and verifier out of it.
// It uses the kyber.Proof framework for building "proofs systems for general statements about discrete logarithms" following
// Camenish and Stadler techniques.
// It also provide the necessary code to interface with the said framework (ProverContext and VerifierContext)
// and ~API-wrappers to use them in a readable way.
// TODO maybe organize those in a "sub package" of daga "ClientProof"
//  + see if we can work from the structure of the ~API-wrapper
//  to put a reusable thing in kyber.proof inspired from it, to ease future usage of the framework and/or to add documentation.

import (
	"errors"
	"fmt"
	"go.dedis.ch/kyber"
	"go.dedis.ch/kyber/proof"
	"strconv"
)

// Sigma-protocol proof.VerifierContext used to conduct interactive proofs PKclient with a prover (daga client)
// meant to be used to interface between user-code (see verifyClientProof() function) and a proof.Verifier built for the
// ClientProofPredicate (see newClientProofPred()).
type clientVerifierCtx struct {
	SuiteProof
	commitsChan       chan kyber.Point    // to give prover's commitments to the proof.Verifier
	challengeChan     chan kyber.Scalar   // to give master challenge to the proof.Verifier (via PubRand)
	subChallengesChan chan []kyber.Scalar // to give the Prover generated sub-challenges to the proof.Verifier (via Get)
	responsesChan     chan kyber.Scalar   // to give prover's responses to the proof.Verifier (via Get)
}

// returns a pointer to a newly allocated and initialized clientVerifierCtx struct
//
// suite is the concrete Suite currently used in this daga implementation
//
// n = #clients in auth. group = #predicates in OrProof
func newClientVerifierCtx(suite Suite, n int) *clientVerifierCtx {
	return &clientVerifierCtx{
		SuiteProof:        newSuiteProof(suite),
		commitsChan:       make(chan kyber.Point, 3*n),  // Point FIFO of size 3*n. user-code - receiveCommitments() -> commitsChan -> Get()* - Verifier
		challengeChan:     make(chan kyber.Scalar),      // Scalar unbuffered chan. user-code - receiveChallenges() -> challengeChan ->  PubRand() - Verifier
		subChallengesChan: make(chan []kyber.Scalar),    // []Scalar unbuffered chan. user-code - receiveChallenges() -> challengeChan -> Get() - Verifier
		responsesChan:     make(chan kyber.Scalar, 2*n), // Scalar FIFO of size 2*n. user-code - receiveResponses() -> responsesChan -> Get()* - Verifier
	}
}

// called by proof.Verifier to receive message from user-code
// satisfy the proof.VerifierContext interface,
// this method is not meant to be used by "user" code, instead see receiveCommitments, receiveCHallenges and receiveResponses methods.
// but is used internally as a user-provided callback by the proof.Verifier (proof framework) to receive various data.
// see the comments and the documentation of the proof framework
func (cvCtx clientVerifierCtx) Get(message interface{}) error {
	switch msg := message.(type) {
	case kyber.Point:
		// Verifier want to receive a commitment
		// receive commitment from user code (via commits channel via receiveCommitments method)
		commitment := <-cvCtx.commitsChan
		msg.Set(commitment)
	case []kyber.Scalar:
		// Verifier want to receive a slice of all n sub-challenges
		// receive sub-challenges from user code (via subChallenges channel via receiveChallenges method)
		// blocks until challenges sent in channel by user code (via receiveChallenges method)
		subChallenges := <-cvCtx.subChallengesChan
		for i, subChallenge := range subChallenges {
			msg[i] = subChallenge // QUESTION use msg[i].Set() instead ?
		}
	case kyber.Scalar:
		// Verifier wants to receive a response
		// receive response from user code (via responses channel via responses method)
		response := <-cvCtx.responsesChan
		msg.Set(response)
	default:
		return errors.New("clientVerifierCtx.Get: message from verifier not of type kyber.Point neither kyber.Scalar " +
			"nor []kyber.Scalar (" + fmt.Sprintf("%T", message) + ")")
	}
	return nil
}

// used to provide the commitments to the running proof.Verifier
func (cvCtx clientVerifierCtx) receiveCommitments(commitments []kyber.Point) error {
	if len(commitments) != cap(cvCtx.commitsChan) {
		return errors.New("clientVerifierCtx.receiveCommitments: wrong number of commitments (" +
			strconv.Itoa(len(commitments)) + ") expected " + strconv.Itoa(cap(cvCtx.commitsChan)))
	}

	// Verifier start by calling 3*n times (len(commitments)) Get() to obtain the commitments from the Prover
	for _, commit := range commitments {
		cvCtx.commitsChan <- commit // blocks if chan full which never happen (buffer has the right size (3*#clients/predicates in the OrPred))
	}
	return nil
}

// used to provide the responses to the running proof.Verifier
func (cvCtx clientVerifierCtx) receiveResponses(responses []kyber.Scalar) error {
	if len(responses) != cap(cvCtx.responsesChan) {
		return errors.New("clientVerifierCtx.receiveResponses: wrong number of responses (" +
			strconv.Itoa(len(responses)) + ") expected " + strconv.Itoa(cap(cvCtx.responsesChan)))
	}

	// Verifier calls Get() 2*n times to obtain the responses
	for _, response := range responses {
		cvCtx.responsesChan <- response // blocks if chan full which never happen (buffer has the right size (2*#clients/predicates in the OrPred))
	}
	return nil
}

// used to provide the challenges to the running proof.Verifier
func (cvCtx clientVerifierCtx) receiveChallenges(challenge kyber.Scalar, subChallenges []kyber.Scalar) {
	// IMPORTANT recall that somewhere caller needs to check that the challenge is indeed the same that the one that
	// was collectively generated by the servers and sent during proof generation / (sigma-protocol run)
	cvCtx.challengeChan <- challenge
	cvCtx.subChallengesChan <- subChallenges
}

// called by the proof.Verifier to obtain the master challenge from user code
// satisfy the proof.VerifierContext interface,
// this method is not meant to be used by "user" code, instead see receiveChallenges method.
// but is used internally as a user-provided callback by the proof.Verifier (proof framework) to receive public randomness.
// see the comments and the documentation of the proof framework
func (cvCtx clientVerifierCtx) PubRand(message ...interface{}) error {
	if len(message) != 1 {
		return errors.New("clientVerifierCtx.PubRand called with less or more than one arg, this is not expected")
	}

	// get challenge from user-code (via challenge channel via receiveChallenge method)
	// blocks until challenge received from remote verifier and sent in channel by user code (via receiveChallenge method)
	challenge := <-cvCtx.challengeChan

	switch msg := message[0].(type) {
	case kyber.Scalar:
		msg.Set(challenge)
		return nil
	default:
		return errors.New("clientVerifierCtx.PubRand called with type " + fmt.Sprintf("%T", message[0]) + " instead of kyber.Scalar")
	}
}

// Sigma-protocol proof.ProverContext used to conduct interactive proofs PKclient with a verifier (daga server)
// meant to be used to interface between user-code (see newClientProof() function) and a proof.Prover built for the
// ClientProofPredicate (see newClientProofPred()).
type clientProverCtx struct {
	SuiteProof
	commitsChan       chan kyber.Point    // to extract the prover's commitments from Prover (via Put) and make them accessible
	challengeChan     chan kyber.Scalar   // to give master challenge to the Prover (via PubRand) and make them accessible
	subChallengesChan chan []kyber.Scalar // to extract the prover's sub-challenges from Prover (via Put) and make them accessible
	responsesChan     chan kyber.Scalar   // to extract the prover's responses from Prover (via Put) and make them accessible
}

// returns a pointer to a newly allocated and initialized newClientProverCtx struct
//
// suite is the concrete Suite currently used in this daga implementation
//
// n = #clients in auth. group = #predicates in OrProof
func newClientProverCtx(suite Suite, n int) *clientProverCtx {
	return &clientProverCtx{
		SuiteProof:        newSuiteProof(suite),
		commitsChan:       make(chan kyber.Point, 3*n),  // Point FIFO of size 3*n. Prover - Put()* -> commitsChan -> commitments() - user-code
		challengeChan:     make(chan kyber.Scalar),      // Scalar unbuffered chan. user-code - receiveChallenge() -> challengeChan -> PubRand() - Prover
		subChallengesChan: make(chan []kyber.Scalar),    // []Scalar unbuffered chan. Prover - Put() -> subChallengesChan -> receiveChallenge() - user-code
		responsesChan:     make(chan kyber.Scalar, 2*n), // Scalar FIFO of size 2*n. Prover - Put()* -> responsesChan -> responses() - user-code
	}
}

// make the Prover's messages available to our/user code
// called by proof.Prover to send messages to our user-code
// satisfy the proof.ProverContext interface,
// this method is not meant to be used by "user" code, instead see commitments, receiveChallenges and responses methods
// but is used internally as a user-provided callback by the proof.Prover (proof framework) to send various data.
// see the comments and the documentation of the proof framework
func (cpCtx clientProverCtx) Put(message interface{}) error {
	switch msg := message.(type) {
	case kyber.Point:
		// received message is a commitment
		// send commitment to user code (via commits channel via commitments method)
		cpCtx.commitsChan <- msg // blocks if chan full which should never happen (buffer should have the right size (3*#clients/predicates in the OrPred))
	case []kyber.Scalar:
		// received message is a slice of all n sub-challenges
		// send sub-challenges to user code (via subChallenges channel via receiveChallenge method)
		cpCtx.subChallengesChan <- msg // blocks until user code received them (sync: "recv happens before send completes")
	case kyber.Scalar:
		// received message is a response
		// send response to user code (via responses channel via responses method)
		cpCtx.responsesChan <- msg // block if chan full which should never happen (buffer should have the right size, (2*#clients/predicates in the OrPred))
	default:
		return errors.New("clientProverCtx.Put: message from prover not of type kyber.Point neither kyber.Scalar nor []kyber.Scalar (" + fmt.Sprintf("%T", message) + ")")
	}
	return nil
}

// used to retrieve the first messages (commitments) t=(t1.0, t1.10, t1.11,..., tn.0, tn.10, tn.11 ) from the running proof.Prover
func (cpCtx clientProverCtx) commitments() ([]kyber.Point, error) {
	commitments := make([]kyber.Point, 0, cap(cpCtx.commitsChan))
	for commit := range cpCtx.commitsChan {
		// get commitment from Prover (via commitsChan channel via Put method)
		commitments = append(commitments, commit)
	} // blocks if chan empty (should not be a problem), (and until chan closed by sending side when done (closed by Prover in PubRand()))
	// TODO maybe add a watchdog that will return/log an error if blocked too long  ? (because this should never happen !)
	//  in fact good idea but not here, add a regression test case

	if len(commitments) != cap(cpCtx.commitsChan) {
		return nil, errors.New("clientProverCtx.commitments: received wrong number of commitments (" +
			strconv.Itoa(len(commitments)) + ") expected " + strconv.Itoa(cap(cpCtx.commitsChan)))
	}
	return commitments, nil
}

// used to retrieve the responses r=(r1.0, r1.1,..., rn.0, rn.1) from the running proof.Prover
func (cpCtx clientProverCtx) responses() ([]kyber.Scalar, error) {
	responses := make([]kyber.Scalar, 0, cap(cpCtx.responsesChan))
	for response := range cpCtx.responsesChan {
		// get response from Prover (via responsesChan channel via Put method)
		responses = append(responses, response)
	} // blocks if chan empty (should not be a problem), (and until chan closed by sending side when done (when Prover.prove done))
	// TODO maybe add a watchdog that will return an error if blocked too long  ? (because this should never happen !)
	//  in fact good idea but not here, add a regression test case

	if len(responses) != cap(cpCtx.responsesChan) {
		return nil, errors.New("clientProverCtx.responses: received wrong number of responses (" +
			strconv.Itoa(len(responses)) + ") expected " + strconv.Itoa(cap(cpCtx.responsesChan)))
	}
	return responses, nil
}

// called by the proof.Prover to obtain public randomness (master challenge) from user code
// satisfy the proof.ProverContext interface,
// this method is not meant to be used by "user" code, instead see receiveChallenge method.
// but is used internally as a user-provided callback by the proof.Prover (proof framework) to receive public randomness.
// see the comments and the documentation of the proof framework
func (cpCtx clientProverCtx) PubRand(message ...interface{}) error {
	if len(message) != 1 {
		return errors.New("clientProverCtx.PubRand called with less or more than one arg, this is not expected")
	}
	// Prover is done sending the commits with Put => release sync barrier with user code/commitments() method by closing the channel
	close(cpCtx.commitsChan)

	// get challenge from remote verifier (via challenge channel via receiveChallenge method)
	// blocks until challenge received from remote verifier and sent in channel by user code (via receiveChallenge method)
	challenge := <-cpCtx.challengeChan

	switch scalar := message[0].(type) {
	case kyber.Scalar:
		scalar.Set(challenge)
		return nil
	default:
		return errors.New("clientProverCtx.PubRand called with type " + fmt.Sprintf("%T", message[0]) + " instead of kyber.Scalar")
	}
}

// used to send master challenge to the running proof.Prover
func (cpCtx clientProverCtx) receiveChallenges(challenge kyber.Scalar) []kyber.Scalar {
	// send master challenge to Prover (via challenge channel via PubRand method) => release sync barrier with PubRand()
	cpCtx.challengeChan <- challenge // blocks until Prover received the master challenge (sync: "recv happens before send completes")

	// receive sub-challenges
	subChallenges := <-cpCtx.subChallengesChan
	return subChallenges
}

// called by the proof.Prover to obtain private randomness from user code
// satisfy the proof.ProverContext interface,
// this method is not meant to be used by "user" code but is used internally as a user-provided callback
// by the proof.Prover (proof framework) to receive private randomness.
// see the comments and the documentation of the proof framework

// IMPORTANT !! need to make sure that random generator properly setup etc.
// !! because if not, and if say, the same commitments are computed for two proof runs,
// the client's private key can be extracted (using same technique as when proving special soundness property)
// => impersonation possible, anonymity and deniability broken etc..
func (cpCtx clientProverCtx) PriRand(message ...interface{}) error {
	if len(message) > 0 {
		switch scalar := message[0].(type) {
		case kyber.Scalar:
			scalar.Pick(cpCtx.RandomStream())
		default:
			return errors.New("clientProverCtx.PriRand called with type " + fmt.Sprintf("%T", message[0]) + " instead of kyber.Scalar")
		}
	}
	return errors.New("clientProverCtx.PriRand called with no arg, this is not expected")
}

// ClientProof stores the client's proof P0 as of "Syta - Identity Management Through Privacy Preserving Aut 4.3.7"
// and obtained after completion of the sigma-protocol with a server.
//
// Cs the challenge that was sent by the server (containing signatures and the master challenge used to generate the sub-challenges and the responses)
//
// T the commitments (first (prover) message of the sigma-protocol)
//
// C all the sub-challenges (one for each sub predicate of the PKclient OR-predicate)
//
// R the responses (final (prover) message of the sigma-protocol)
type ClientProof struct {
	Cs Challenge
	T  []kyber.Point
	C  []kyber.Scalar
	R  []kyber.Scalar
}

// PKclientVerifier function that returns (eventually) a Challenge when provided with commitments
type PKclientVerifier func([]kyber.Point) (Challenge, error)

// builds a new ClientProof (Step 4 of client's protocol) and returns it to caller
func newClientProof(suite Suite, context AuthenticationContext,
	client Client,
	tagAndCommitments initialTagAndCommitments,
	s kyber.Scalar,
	sendCommitsReceiveChallenge PKclientVerifier) (ClientProof, error) {

	if context == nil {
		return ClientProof{}, errors.New("nil context")
	}
	members := context.Members()

	if len(members.X) <= 1 {
		return ClientProof{}, errors.New("newClientProof: there is only one client in the context, this means DAGA is pointless")
		// moreover the following code (and more or less DAGA paper) assumes that there is at least 2 clients/predicates
		// in the context/OR-predicate, if this condition is not met there won't be an "subChallenges" to generate by the
		// prover => he won't send them by calling Put, but we wait for them !!
		// in case this assumption needs to be relaxed, a test should be added to the proverContext.receiveChallenges() method
	}

	//construct the proof.Prover for client's PK predicate and its proof.ProverContext
	prover := newClientProver(suite, context, tagAndCommitments, client, s)
	proverCtx := newClientProverCtx(suite, len(members.X))

	//3-move interaction with server
	//	start the proof.Prover and proof machinery in new goroutine
	var P ClientProof
	var proverErr error
	go func() {
		defer close(proverCtx.responsesChan)
		proverErr = prover(proverCtx)
	}()

	//	get initial commitments from running Prover
	if commits, err := proverCtx.commitments(); err != nil {
		return ClientProof{}, errors.New("newClientProof:" + err.Error())
	} else {
		P.T = commits
	}

	//	forward them to random remote server/verifier (over *anon.* circuit etc.. concern of the caller code / client setup!!)
	//	and receive master challenge from remote server(s) (over *anon.* circuit etc.. concern of the caller code / client setup!!)
	challenge, err := sendCommitsReceiveChallenge(P.T)
	if err != nil {
		// TODO kill prover gorountine... but I'll argue that this is useless since this code is running clientside and the process will terminate on error
		return ClientProof{}, errors.New("newClientProof: failed to receive challenge: " + err.Error())
	}

	if err := challenge.VerifySignatures(suite, members.Y, P.T); err != nil {
		// TODO kill prover gorountine... but I'll argue that this is useless since this code is running clientside and the process will terminate on error
		return ClientProof{}, errors.New("newClientProof:" + err.Error())
	}
	P.Cs = challenge

	//	forward master challenge to running Prover in order to continue the proof process, and receive the sub-challenges from Prover
	P.C = proverCtx.receiveChallenges(P.Cs.Cs)

	//	get final responses from Prover
	if responses, err := proverCtx.responses(); err != nil {
		return ClientProof{}, errors.New("newClientProof:" + err.Error())
	} else {
		P.R = responses
	}

	//check return value of the now done proof.Prover
	if proverErr != nil { // here no race, we are sure that Prover is done since responses() returns only after response chan is closed
		return ClientProof{}, errors.New("newClientProof:" + proverErr.Error())
	}
	return P, nil
}

// verifyClientProof checks the validity of a client's ClientProof
func verifyClientProof(suite Suite, context AuthenticationContext,
	proof ClientProof,
	tagAndCommitments initialTagAndCommitments) error {

	if context == nil {
		return errors.New("verifyClientProof: nil context")
	}
	members := context.Members()

	if len(members.X) <= 1 {
		return errors.New("verifyClientProof: there is only one client in the context, this means DAGA is pointless")
		// moreover the following code (and more or less DAGA paper) assumes that there is at least 2 clients/predicates
		// in the context/OR-predicate, if this condition is not met there won't be any "subChallenges" to request by the
		// verifier => he won't call Get to receive them
		// in case this assumption needs to be relaxed, a test should be added to the verifierContext.receiveChallenges() method
	}

	// verify that commitments and challenge sent in proof transcript are "genuine"
	// i.e. don't blindly trust the challenge and commitments sent with proof,
	// need to ensure that they are the same commitments/challenge that were
	// sent during the sigma-protocol run / when client-prover requested the "collective honest random challenge"
	if err := proof.Cs.VerifySignatures(suite, members.Y, proof.T); err != nil {
		return errors.New("verifyClientProof: proof transcript not accepted, commitments or challenge mismatch")
	}

	//construct the proof.Verifier for client's PK and its proof.VerifierContext
	verifier := newClientVerifier(suite, context, tagAndCommitments)
	verifierCtx := newClientVerifierCtx(suite, len(members.X))

	//3-move interaction with client
	//	start the proof.Verifier and proof machinery in new goroutine
	verifierErrChan := make(chan error)
	go func() {
		verifierErrChan <- verifier(verifierCtx)
	}()

	//	forward commitments to running Verifier
	commitments := proof.T

	if err := verifierCtx.receiveCommitments(commitments); err != nil {
		return errors.New("verifyClientProof:" + err.Error())
	}

	//	forward challenges to running Verifier
	challenge := proof.Cs.Cs
	subChallenges := proof.C
	verifierCtx.receiveChallenges(challenge, subChallenges)

	//	forward responses to running Verifier
	responses := proof.R
	if err := verifierCtx.receiveResponses(responses); err != nil {
		return errors.New("verifyClientProof:" + err.Error())
	}

	//wait for Verifier to be done and check return value of the now done proof.Verifier
	verifierErr := <-verifierErrChan
	if verifierErr != nil { // here no race, we are sure that Verifier is done since responses() returns only after response chan is closed
		return errors.New("verifyClientProof:" + verifierErr.Error())
	}
	return nil
}

// returns a new proof.Predicate that hold the PKclient predicate (being proven by the daga clients as part of their auth. process)
// it is used by the kyber.proof framework to generate Provers and Verifiers.
// see 4.3.7 Client’s Proof PKclient
//
// context the AuthenticationContext
//
// tagAndCommitments the initialTagAndCommitments of a client (see initialTagAndCommitments)
func newClientProofPred(suite Suite, context AuthenticationContext, tagAndCommitments initialTagAndCommitments) (proof.Predicate, map[string]kyber.Point) {
	members := context.Members()
	// build the OR-predicate
	andPreds := make([]proof.Predicate, 0, len(members.X))

	// map for public values needed to construct the Prover and Verifier from the predicate
	pval := make(map[string]kyber.Point, 3+2*len(members.X))
	pval["G"] = suite.Point().Base()
	pval["T0"] = tagAndCommitments.T0
	pval["Sm"] = tagAndCommitments.SCommits[len(tagAndCommitments.SCommits)-1]

	//	build all the internal And predicates (one for each client in current auth. group)
	for k, pubKey := range members.X {
		// client AndPred
		kStr := strconv.Itoa(k)
		//		i) client i’s linkage tag T0 is created with respect to his per-round generator hi
		linkageTagValidPred := proof.Rep("T0", "s", "H"+kStr)
		// 		ii)  S is a proper commitment to the product of all secrets that i shares with the servers
		commitmentValidPred := proof.Rep("Sm", "s", "G")
		// 		iii) client i's private key xi corresponds to one of the public keys included in the group definition G
		knowOnePrivateKeyPred := proof.Rep("X"+kStr, "x"+kStr, "G")

		clientAndPred := proof.And(knowOnePrivateKeyPred, commitmentValidPred, linkageTagValidPred)

		// update map of public values
		pval["X"+kStr] = pubKey
		pval["H"+kStr] = context.ClientsGenerators()[k]

		andPreds = append(andPreds, clientAndPred)
	}
	finalOrPred := proof.Or(andPreds...)

	return finalOrPred, pval
}

// returns a proof.Verifier for PKclient
//
// context the AuthenticationContext (used to build the PKclient predicate)
//
// tagAndCommitments the initialTagAndCommitments sent by the client that generated the proof we want to verify (see initialTagAndCommitments)
func newClientVerifier(suite Suite, context AuthenticationContext, tagAndCommitments initialTagAndCommitments) proof.Verifier {
	// build OR-predicate of the client proof, and the map of public values
	finalOrPred, pval := newClientProofPred(suite, context, tagAndCommitments)

	// retrieve sigma-protocol Verifier for the OR-predicate
	return finalOrPred.Verifier(newSuiteProof(suite), pval)
}

// returns a proof.Prover for PKclient
//
// context the AuthenticationContext
//
// tagAndCommitments the initialTagAndCommitments of the client (see initialTagAndCommitments)
//
// client the Client
//
// s the opening (multiplication of all shared secrets) of Sm (tagAndCommitments.sCommits[len(tagAndCommitments.sCommits)-1])
func newClientProver(suite Suite, context AuthenticationContext, tagAndCommitments initialTagAndCommitments, client Client, s kyber.Scalar) proof.Prover {
	// build OR-predicate of the client proof, and the map of public values
	finalOrPred, pval := newClientProofPred(suite, context, tagAndCommitments)

	// build map of secret values and choice needed to construct the Prover from the predicate
	choice := map[proof.Predicate]int{
		finalOrPred: client.Index(), // indicate to prover which clause is actually true
	}
	sval := map[string]kyber.Scalar{
		"s":                                s,
		"x" + strconv.Itoa(client.Index()): client.PrivateKey(),
	}
	// retrieve sigma-protocol Prover for the OR-predicate
	prover := finalOrPred.Prover(newSuiteProof(suite), sval, pval, choice)
	return prover
}

//ToBytes is a helper function used to marshal a ClientProof into []byte to be used in signatures
func (proof ClientProof) ToBytes() (data []byte, err error) {
	data, e := proof.Cs.Cs.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("error marshalling cs: %s", e)
	}

	temp, e := PointArrayToBytes(proof.T)
	if e != nil {
		return nil, fmt.Errorf("error marshalling t: %s", e)
	}
	data = append(data, temp...)

	temp, e = ScalarArrayToBytes(proof.C)
	if e != nil {
		return nil, fmt.Errorf("error marshalling c: %s", e)
	}
	data = append(data, temp...)

	temp, e = ScalarArrayToBytes(proof.R)
	if e != nil {
		return nil, fmt.Errorf("error marshalling r: %s", e)
	}
	data = append(data, temp...)

	return data, nil
}
