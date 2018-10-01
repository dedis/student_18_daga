package daga

import (
	"crypto/sha512"
	"fmt"
	"github.com/dedis/kyber"
	"io"
	"strconv"
)

/*Server is used to store the server's private key and index.
All the server's methods are attached to it */
type Server struct {
	private kyber.Scalar
	index   int
	r       kyber.Scalar //Per round secret
}

/*Commitment stores the index of the server, the commitment value and the signature for the commitment*/
type Commitment struct {
	commit kyber.Point
	sig    serverSignature
}

/*serverSignature stores a signature created by a server and the server's index*/
type serverSignature struct {
	index int
	sig   []byte
}

/*ChallengeCheck stores all the information passed along the servers to check and sign the challenge*/
type ChallengeCheck struct {
	cs       kyber.Scalar
	sigs     []serverSignature //Signatures for cs only
	commits  []Commitment
	openings []kyber.Scalar
}

/*Challenge stores the collectively generated challenge and the signatures of the servers
This is the structure sent to the client*/
type Challenge struct {
	cs   kyber.Scalar
	sigs []serverSignature
}

/*ServerMessage stores the message sent by a server to one or many others*/
type ServerMessage struct {
	request ClientMessage
	tags    []kyber.Point
	proofs  []serverProof
	indexes []int
	sigs    []serverSignature
}

/*serverProof stores a server proof of his computations*/
type serverProof struct {
	t1 kyber.Point
	t2 kyber.Point
	t3 kyber.Point
	c  kyber.Scalar
	r1 kyber.Scalar
	r2 kyber.Scalar
}

//CreateServer is used to initialize a new server with a given index
//If no private key is given, a random one is chosen
func CreateServer(i int, s kyber.Scalar) (server Server, err error) {
	if i < 0 {
		return Server{}, fmt.Errorf("Invalid parameters")
	}
	if s == nil {
		s = suite.Scalar().Pick(suite.RandomStream())
	}
	return Server{index: i, private: s, r: nil}, nil
}

//GetPublicKey returns the public key associated with a server
func (server *Server) GetPublicKey() kyber.Point {
	return suite.Point().Mul(server.private, nil)
}

/*GenerateCommitment creates the commitment and its opening for the distributed challenge generation*/
func (server *Server) GenerateCommitment(context *ContextEd25519) (commit *Commitment, opening kyber.Scalar, err error) {
	opening = suite.Scalar().Pick(suite.RandomStream())
	com := suite.Point().Mul(opening, nil)
	msg, err := com.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("Error in conversion of commit: %s", err)
	}
	sig, err := ECDSASign(server.private, msg)
	if err != nil {
		return nil, nil, fmt.Errorf("Error in commit signature generation: %s", err)
	}
	return &Commitment{sig: serverSignature{index: server.index, sig: sig}, commit: com}, opening, nil
}

/*VerifyCommitmentSignature verifies that all the commitments are valid and correctly signed*/
func VerifyCommitmentSignature(context *ContextEd25519, commits []Commitment) (err error) {
	for i, com := range commits {
		if i != com.sig.index {
			return fmt.Errorf("Wrong index: got %d expected %d", com.sig.index, i)
		}
		// QUESTION FIXME: How to check that a point is on the curve?

		//Convert the commitment and verify the signature
		msg, e := com.commit.MarshalBinary()
		if e != nil {
			return fmt.Errorf("Error in conversion of commit for verification: %s", err)
		}
		err = ECDSAVerify(context.G.Y[i], msg, com.sig.sig)
		if err != nil {
			return err
		}
	}
	return nil
}

/*CheckOpenings verifies each opening and returns the computed challenge*/
func CheckOpenings(context *ContextEd25519, commits []Commitment, openings []kyber.Scalar) (cs kyber.Scalar, err error) {
	if context == nil {
		return nil, fmt.Errorf("Empty context")
	}
	if len(commits) != len(context.G.Y) {
		return nil, fmt.Errorf("Incorrect number of commits: got %d expected %d", len(commits), len(context.G.Y))
	}
	if len(openings) != len(context.G.Y) {
		return nil, fmt.Errorf("Incorrect number of openings: got %d expected %d", len(openings), len(context.G.Y))
	}

	cs = suite.Scalar().Zero()
	for i := 0; i < len(commits); i++ {
		c := suite.Point().Mul(openings[i], nil)
		if !commits[i].commit.Equal(c) {
			return nil, fmt.Errorf("Mismatch opening for server %d", i)
		}
		cs = suite.Scalar().Add(cs, openings[i])
	}
	return cs, nil
}

/*InitializeChallenge creates a Challenge structure from a challenge value
It checks the openings before doing so*/
func InitializeChallenge(context *ContextEd25519, commits []Commitment, openings []kyber.Scalar) (*ChallengeCheck, error) {
	if context == nil || commits == nil || openings == nil || len(commits) == 0 || len(openings) == 0 || len(commits) != len(openings) {
		return nil, fmt.Errorf("Invalid inputs")
	}
	cs, err := CheckOpenings(context, commits, openings)
	if err != nil {
		return nil, err
	}

	return &ChallengeCheck{cs: cs, commits: commits, openings: openings, sigs: nil}, nil
}

/*CheckUpdateChallenge verifies that all the previous servers computed the same challenges and that their signatures are valid
It also adds the server's signature to the list if the round-robin is not completed (the challenge has not yet made it back to the leader)
It must be used after the leader ran InitializeChallenge and after each server received the challenge from the previous server*/
func (server *Server) CheckUpdateChallenge(context *ContextEd25519, challenge *ChallengeCheck) error {
	//Check the signatures and check for duplicates
	msg, e := challenge.cs.MarshalBinary()
	if e != nil {
		return fmt.Errorf("Error in challenge conversion: %s", e)
	}
	encountered := map[int]bool{}
	for _, sig := range challenge.sigs {
		if encountered[sig.index] == true {
			return fmt.Errorf("Duplicate signature")
		}
		encountered[sig.index] = true

		e = ECDSAVerify(context.G.Y[sig.index], msg, sig.sig)
		if e != nil {
			return fmt.Errorf("%s", e)
		}
	}

	//Checks the signatures of the commitments
	err := VerifyCommitmentSignature(context, challenge.commits)
	if err != nil {
		return err
	}
	//Checks the openings
	cs, err := CheckOpenings(context, challenge.commits, challenge.openings)
	if err != nil {
		return err
	}
	//Checks that the challenge values match
	if !cs.Equal(challenge.cs) {
		return fmt.Errorf("Challenge values does not match")
	}

	//Add the server's signature to the list if it is not the last one
	if len(challenge.sigs) == len(context.G.Y) {
		return nil
	}
	sig, e := ECDSASign(server.private, msg)
	if e != nil {
		return e
	}
	challenge.sigs = append(challenge.sigs, serverSignature{index: server.index, sig: sig})

	return nil
}

/*FinalizeChallenge is used to convert the data passed between the servers into the challenge sent to the client
It must be used after the leader got the message back and ran CheckUpdateChallenge*/
func FinalizeChallenge(context *ContextEd25519, challenge *ChallengeCheck) (*Challenge, error) {
	if context == nil || challenge == nil {
		return nil, fmt.Errorf("Invalid inputs")
	}
	if len(challenge.sigs) != len(context.G.Y) {
		return nil, fmt.Errorf("Signature count does not match: got %d expected %d", len(challenge.sigs), len(context.G.Y))
	}

	return &Challenge{cs: challenge.cs, sigs: challenge.sigs}, nil
}

//InitializeServerMessage creates a ServerMessage from a ClientMessage to ease further processing
func (server *Server) InitializeServerMessage(request *ClientMessage) (msg *ServerMessage) {
	if request == nil {
		return nil
	}
	return &ServerMessage{request: *request, tags: nil, indexes: nil, proofs: nil, sigs: nil}
}

/*ServerProtocol runs the server part of DAGA upon receiving a message from either a server or a client*/
func (server *Server) ServerProtocol(context *ContextEd25519, msg *ServerMessage) error {
	//Step 1
	//Verify that the message is correctly formed
	if !ValidateClientMessage(&msg.request) {
		return fmt.Errorf("Invalid client's request")
	}
	if len(msg.indexes) != len(msg.proofs) || len(msg.proofs) != len(msg.tags) || len(msg.tags) != len(msg.sigs) {
		return fmt.Errorf("Invalid message")
	}

	//Checks that not all servers already did the protocol
	if len(msg.indexes) >= len(context.G.Y) {
		return fmt.Errorf("Too many calls of the protocol")
	}

	// Iteratively checks each signature if this is not the first server to receive the client's request
	data, e := msg.request.ToBytes()
	if e != nil {
		return fmt.Errorf("Error in request: %s", e)
	}
	if len(msg.indexes) != 0 {
		for i := 0; i < len(msg.indexes); i++ {
			temp, err := msg.tags[i].MarshalBinary()
			if err != nil {
				return fmt.Errorf("Error in tags: %s", err)
			}
			data = append(data, temp...)

			temp, err = msg.proofs[i].ToBytes()
			if err != nil {
				return fmt.Errorf("Error in proofs: %s", err)
			}
			data = append(data, temp...)

			data = append(data, []byte(strconv.Itoa(msg.indexes[i]))...)

			err = ECDSAVerify(context.G.Y[msg.sigs[i].index], data, msg.sigs[i].sig)
			if err != nil {
				return fmt.Errorf("Error in signature: "+strconv.Itoa(i)+"\n%s", err)
			}
		}
	}

	// Check the client proof
	if !verifyClientProof(msg.request) {
		return fmt.Errorf("Invalid client's proof")
	}

	//Check all the proofs
	if len(msg.proofs) != 0 {
		for i, p := range msg.proofs {
			var valid bool
			if p.r2 == nil {
				valid = verifyMisbehavingProof(context, i, &p, msg.request.sArray[0])
			} else {
				valid = verifyServerProof(context, i, msg)
			}
			if !valid {
				return fmt.Errorf("Invalid server proof")
			}
		}
	}

	//Step 2: Verify the correct behaviour of the client
	hasher := sha512.New()
	var writer io.Writer = hasher // same QUESTION WTF not needed
	suite.Point().Mul(server.private, msg.request.sArray[0]).MarshalTo(writer)
	hash := hasher.Sum(nil)
	hasher = suite.Hash()
	hasher.Write(hash)
	s := suite.Scalar().SetBytes(hasher.Sum(nil))
	//rand := suite.Cipher(hash)  // QUESTION here it is again logical but.. again WTF?
	//s := suite.Scalar().Pick(rand)
	var T kyber.Point
	var proof *serverProof
	//Detect a misbehaving client and generate the elements of the server's message accordingly
	if !msg.request.sArray[server.index+2].Equal(suite.Point().Mul(s, msg.request.sArray[server.index+1])) {
		T = suite.Point().Null()
		proof, e = server.generateMisbehavingProof(context, msg.request.sArray[0])
	} else {
		inv := suite.Scalar().Inv(s)
		exp := suite.Scalar().Mul(server.r, inv)
		if len(msg.tags) == 0 {
			T = suite.Point().Mul(exp, msg.request.t0)
		} else {
			T = suite.Point().Mul(exp, msg.tags[len(msg.tags)-1])
		}
		proof, e = server.generateServerProof(context, s, T, msg)
	}
	if e != nil {
		return e
	}

	//Signs our message
	temp, e := T.MarshalBinary()
	if e != nil {
		return fmt.Errorf("Error in T: %s", e)
	}
	data = append(data, temp...)

	temp, e = proof.ToBytes()
	if e != nil {
		return fmt.Errorf("Error in proof: %s", e)
	}
	data = append(data, temp...)

	data = append(data, []byte(strconv.Itoa(server.index))...)

	sign, e := ECDSASign(server.private, data)
	if e != nil {
		return fmt.Errorf("Error in own signature: %s", e)
	}

	signature := serverSignature{sig: sign, index: server.index}

	//Step 4: Form the new message
	msg.tags = append(msg.tags, T)
	msg.proofs = append(msg.proofs, *proof)
	msg.indexes = append(msg.indexes, server.index)
	msg.sigs = append(msg.sigs, signature)

	return nil
}

/*generateServerProof creates the server proof for its computations*/
func (server *Server) generateServerProof(context *ContextEd25519, s kyber.Scalar, T kyber.Point, msg *ServerMessage) (proof *serverProof, err error) {
	//Input validation
	if context == nil {
		return nil, fmt.Errorf("Empty context")
	}
	if s == nil {
		return nil, fmt.Errorf("Empty s")
	}
	if T == nil {
		return nil, fmt.Errorf("Empty T")
	}
	if msg == nil {
		return nil, fmt.Errorf("Empty server message")
	}

	//Step 1
	v1 := suite.Scalar().Pick(suite.RandomStream())
	v2 := suite.Scalar().Pick(suite.RandomStream())

	var a kyber.Point
	if len(msg.tags) == 0 {
		a = suite.Point().Mul(v1, msg.request.t0)
	} else {
		a = suite.Point().Mul(v1, msg.tags[len(msg.tags)-1])
	}

	//exp := suite.Scalar().Neg(v2)
	b := suite.Point().Mul(v2, T)
	t1 := suite.Point().Sub(a, b)

	t2 := suite.Point().Mul(v1, nil)

	t3 := suite.Point().Mul(v2, msg.request.sArray[server.index+1]) //Accesses S[j-1]

	//Step 2
	var Tprevious kyber.Point
	if len(msg.tags) == 0 {
		Tprevious = msg.request.t0
	} else {
		Tprevious = msg.tags[len(msg.tags)-1]
	}
	//Generating the hash
	hasher := sha512.New()
	var writer io.Writer = hasher // QUESTION again..
	//hash guarantees that no error are returned on write, so we do not check for error below // QUESTION agree so why check at other places ?
	Tprevious.MarshalTo(writer)
	T.MarshalTo(writer)
	context.R[server.index].MarshalTo(writer)
	suite.Point().Mul(suite.Scalar().One(), nil).MarshalTo(writer)
	msg.request.sArray[server.index+2].MarshalTo(writer)
	msg.request.sArray[server.index+1].MarshalTo(writer)
	t1.MarshalTo(writer)
	t2.MarshalTo(writer)
	t3.MarshalTo(writer)
	challenge := hasher.Sum(nil)

	hasher = suite.Hash()
	hasher.Write(challenge)
	c := suite.Scalar().SetBytes(hasher.Sum(nil))
	//rand := suite.Cipher(challenge)
	//c := suite.Scalar().Pick(rand)
	//Step 3
	d := suite.Scalar().Mul(c, server.r)
	r1 := suite.Scalar().Sub(v1, d)

	e := suite.Scalar().Mul(c, s)
	r2 := suite.Scalar().Sub(v2, e)

	//Step 4
	return &serverProof{
		t1: t1,
		t2: t2,
		t3: t3,
		c:  c,
		r1: r1,
		r2: r2,
	}, nil
}

/*verifyServerProof verifies a server proof*/
func verifyServerProof(context *ContextEd25519, i int, msg *ServerMessage) bool {
	//Input checks
	if context == nil || msg == nil {
		return false
	}

	if i >= len(msg.proofs) || i < 0 {
		return false
	}

	//Verify format of the proof
	if msg.proofs[i].c == nil || msg.proofs[i].t1 == nil || msg.proofs[i].t2 == nil || msg.proofs[i].t3 == nil || msg.proofs[i].r1 == nil || msg.proofs[i].r2 == nil {
		return false
	}

	index := msg.indexes[i]

	//Step 1
	var a kyber.Point
	if i == 0 {
		a = suite.Point().Mul(msg.proofs[i].r1, msg.request.t0)
	} else {
		a = suite.Point().Mul(msg.proofs[i].r1, msg.tags[i-1])
	}
	//exp := suite.Scalar().Neg(msg.proofs[i].r2)
	b := suite.Point().Mul(msg.proofs[i].r2, msg.tags[i])
	t1 := suite.Point().Sub(a, b)

	d := suite.Point().Mul(msg.proofs[i].r1, nil)
	e := suite.Point().Mul(msg.proofs[i].c, context.R[index])
	t2 := suite.Point().Add(d, e)

	f := suite.Point().Mul(msg.proofs[i].r2, msg.request.sArray[index+1])
	g := suite.Point().Mul(msg.proofs[i].c, msg.request.sArray[index+2])
	t3 := suite.Point().Add(f, g)

	//Step 2
	var Tprevious kyber.Point
	if i == 0 {
		Tprevious = msg.request.t0
	} else {
		Tprevious = msg.tags[i-1]
	}
	hasher := sha512.New()
	var writer io.Writer = hasher
	Tprevious.MarshalTo(writer)
	msg.tags[i].MarshalTo(writer)
	context.R[index].MarshalTo(writer)
	suite.Point().Mul(suite.Scalar().One(), nil).MarshalTo(writer)
	msg.request.sArray[index+2].MarshalTo(writer)
	msg.request.sArray[index+1].MarshalTo(writer)
	t1.MarshalTo(writer)
	t2.MarshalTo(writer)
	t3.MarshalTo(writer)
	challenge := hasher.Sum(nil)

	hasher = suite.Hash()
	hasher.Write(challenge)
	//rand := suite.Cipher(challenge)
	//c := suite.Scalar().Pick(rand)
	c := suite.Scalar().SetBytes(hasher.Sum(nil))

	if !c.Equal(msg.proofs[i].c) {
		return false
	}

	return true
}

/*generateMisbehavingProof creates the proof of a misbehaving client*/ // QUESTION server ? purpose of comment ?
func (server *Server) generateMisbehavingProof(context *ContextEd25519, Z kyber.Point) (proof *serverProof, err error) {
	//Input checks
	if context == nil {
		return nil, fmt.Errorf("Empty context")
	}
	if Z == nil {
		return nil, fmt.Errorf("Empty Z")
	}

	Zs := suite.Point().Mul(server.private, Z) // QUESTION secure (even if the function is called misbehaving whatever) ? ...+ TODO maybe I have missed other parts

	//Step 1
	v := suite.Scalar().Pick(suite.RandomStream())
	t1 := suite.Point().Mul(v, Z)
	t2 := suite.Point().Mul(v, nil)

	//Step 2
	hasher := sha512.New()
	var writer io.Writer = hasher
	Zs.MarshalTo(writer)
	Z.MarshalTo(writer)
	context.G.Y[server.index].MarshalTo(writer)
	suite.Point().Mul(suite.Scalar().One(), nil).MarshalTo(writer)
	t1.MarshalTo(writer)
	t2.MarshalTo(writer)
	challenge := hasher.Sum(nil)

	hasher = suite.Hash()
	hasher.Write(challenge)
	//rand := suite.Cipher(challenge)
	c := suite.Scalar().SetBytes(hasher.Sum(nil))

	//Step 3
	a := suite.Scalar().Mul(c, server.private)
	r := suite.Scalar().Sub(v, a)

	//Step 4
	return &serverProof{
		t1: t1,
		t2: t2,
		t3: Zs,
		c:  c,
		r1: r,
		r2: nil,
	}, nil
}

/*verifyMisbehavingProof verifies a proof of a misbehaving client*/ // QUESTION server ? ..
func verifyMisbehavingProof(context *ContextEd25519, i int, proof *serverProof, Z kyber.Point) bool {
	//Input checks
	if context == nil || proof == nil || Z == nil {
		return false
	}

	if i < 0 || i >= len(context.G.Y) {
		return false
	}

	//Check that this is a misbehaving proof
	if proof.r2 != nil {
		return false
	}

	//Verify format of the proof
	if proof.t1 == nil || proof.t2 == nil || proof.t3 == nil || proof.c == nil || proof.r1 == nil {
		return false
	}

	//Step 1
	a := suite.Point().Mul(proof.r1, Z)       //r1 = r
	b := suite.Point().Mul(proof.c, proof.t3) //t3 = Zs
	t1 := suite.Point().Add(a, b)

	d := suite.Point().Mul(proof.r1, nil) //r1 = r
	e := suite.Point().Mul(proof.c, context.G.Y[i])
	t2 := suite.Point().Add(d, e)

	//Step 2
	hasher := sha512.New()
	var writer io.Writer = hasher
	proof.t3.MarshalTo(writer)
	Z.MarshalTo(writer)
	context.G.Y[i].MarshalTo(writer)
	suite.Point().Mul(suite.Scalar().One(), nil).MarshalTo(writer)
	t1.MarshalTo(writer)
	t2.MarshalTo(writer)
	challenge := hasher.Sum(nil)

	hasher = suite.Hash()
	hasher.Write(challenge)

	//rand := suite.Cipher(challhasenge)
	c := suite.Scalar().SetBytes(hasher.Sum(nil))

	if !c.Equal(proof.c) {
		return false
	}

	return true
}

/*GenerateNewRoundSecret creates a new secret for the server, erasing the previous one.
It returns the commitment to that secret to be included in the context*/
func (server *Server) GenerateNewRoundSecret() (R kyber.Point) {
	server.r = suite.Scalar().Pick(suite.RandomStream()) // TODO see if used like a key...
	return suite.Point().Mul(server.r, nil)
}

/*ToBytes is a helper function used to convert a ServerProof into []byte to be used in signatures*/
// QUESTION WTF ? + DRY there should be another way or no ?
func (proof *serverProof) ToBytes() (data []byte, err error) {
	temp, e := proof.t1.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("Error in t1: %s", e)
	}
	data = append(data, temp...)

	temp, e = proof.t2.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("Error in t2: %s", e)
	}
	data = append(data, temp...)

	temp, e = proof.t3.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("Error in t3: %s", e)
	}
	data = append(data, temp...)

	temp, e = proof.c.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("Error in c: %s", e)
	}
	data = append(data, temp...)

	temp, e = proof.r1.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("Error in r1: %s", e)
	}
	data = append(data, temp...)

	//Need to test if r2 == nil (Misbehaving)
	if proof.r2 != nil {
		temp, e = proof.r2.MarshalBinary()
		if e != nil {
			return nil, fmt.Errorf("Error in r2: %s", e)
		}
		data = append(data, temp...)
	}

	return data, nil
}
