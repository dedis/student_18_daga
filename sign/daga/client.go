package daga

import (
	"crypto/sha512"
	"fmt"
	"io"
	"strconv"

	"gopkg.in/dedis/crypto.v0/abstract"
	"gopkg.in/dedis/crypto.v0/random"
)

/*Client is used to store the client's private key and index.
All the client's methods are attached to it*/
type Client struct {
	private abstract.Scalar
	index   int
}

/*ClientMessage stores an authentication request message sent by the client to an arbitrarily chosen server*/
type ClientMessage struct {
	context ContextEd25519
	sArray  []abstract.Point
	t0      abstract.Point
	proof   ClientProof
}

/*ClientProof stores the client's proof of his computations*/
type ClientProof struct {
	cs abstract.Scalar
	t  []abstract.Point
	c  []abstract.Scalar
	r  []abstract.Scalar
}

//CreateClient is used to initialize a new client with a given index
//If no private key is given, a random one is chosen
func CreateClient(i int, s abstract.Scalar) (client Client, err error) {
	if i < 0 {
		return Client{}, fmt.Errorf("Invalid parameters")
	}
	if s == nil {
		s = suite.Scalar().Pick(random.Stream)
	}
	return Client{index: i, private: s}, nil
}

//GetPublicKey returns the public key associated with a client
func (client *Client) GetPublicKey() abstract.Point {
	return suite.Point().Mul(nil, client.private)
}

/*CreateRequest generates the elements for the authentication request (T0, S) and the generation of the client's proof(s)*/
func (client *Client) CreateRequest(context *ContextEd25519) (T0 abstract.Point, S []abstract.Point, s abstract.Scalar, err error) {
	//Step 1: generate ephemeral DH keys
	z := suite.Scalar().Pick(random.Stream)
	Z := suite.Point().Mul(nil, z)

	//Step 2: Generate shared secrets with the servers
	shared := make([][]byte, len(context.G.Y))
	for i := 0; i < len(context.G.Y); i++ {
		hasher := sha512.New()
		var writer io.Writer = hasher
		_, err := suite.Point().Mul(context.G.Y[i], z).MarshalTo(writer)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("Error in shared secrets: %s", err)
		}
		hash := hasher.Sum(nil)
		shared[i] = hash[:]
	}

	//Step 3: initial linkage tag and commitments
	//Computes the value of the exponent for the initial linkage tag
	exp := suite.Scalar().One()
	for i := 0; i < len(context.G.Y); i++ {
		rand := suite.Cipher(shared[i])
		exp.Mul(exp, suite.Scalar().Pick(rand))
	}
	T0 = suite.Point().Mul(context.H[client.index], exp)

	//Computes the commitments
	S = make([]abstract.Point, len(context.G.Y)+1)
	exp = suite.Scalar().One()
	for i := 0; i < len(context.G.Y)+1; i++ {
		S[i] = suite.Point().Mul(nil, exp)
		if i != len(context.G.Y) {
			rand := suite.Cipher(shared[i])
			exp.Mul(exp, suite.Scalar().Pick(rand))
		}
	}
	s = exp

	//Add the client's ephemeral public key to the commitments
	/*Prepend taken from comment at
	https://codingair.wordpress.com/2014/07/18/go-appendprepend-item-into-slice/ */
	S = append(S, nil)
	copy(S[1:], S)
	S[0] = Z

	return T0, S, s, nil
}

//GenerateProofCommitments creates and returns the client's commitments t and the random wieghts w
func (client *Client) GenerateProofCommitments(context *ContextEd25519, T0 abstract.Point, s abstract.Scalar) (t *[]abstract.Point, v, w *[]abstract.Scalar) {
	//Generates w randomly except for w[client.index] = 0
	wtemp := make([]abstract.Scalar, len(context.H))
	w = &wtemp
	for i := range *w {
		(*w)[i] = suite.Scalar().Pick(random.Stream)
	}
	(*w)[client.index] = suite.Scalar().Zero()

	//Generates random v (2 per client)
	vtemp := make([]abstract.Scalar, 2*len(context.H))
	v = &vtemp
	for i := 0; i < len(*v); i++ {
		(*v)[i] = suite.Scalar().Pick(random.Stream)
	}

	//Generates the commitments t (3 per clients)
	ttemp := make([]abstract.Point, 3*len(context.H))
	t = &ttemp
	for i := 0; i < len(context.H); i++ {
		a := suite.Point().Mul(context.G.X[i], (*w)[i])
		b := suite.Point().Mul(nil, (*v)[2*i])
		(*t)[3*i] = suite.Point().Add(a, b)

		Sm := suite.Point().Mul(nil, s)
		c := suite.Point().Mul(Sm, (*w)[i])
		d := suite.Point().Mul(nil, (*v)[2*i+1])
		(*t)[3*i+1] = suite.Point().Add(c, d)

		e := suite.Point().Mul(T0, (*w)[i])
		f := suite.Point().Mul(context.H[i], (*v)[2*i+1])
		(*t)[3*i+2] = suite.Point().Add(e, f)
	}

	return t, v, w
}

//GenerateProofResponses creates the responses to the challenge cs sent by the servers
func (client *Client) GenerateProofResponses(context *ContextEd25519, s abstract.Scalar, challenge *Challenge, v, w *[]abstract.Scalar) (c, r *[]abstract.Scalar, err error) {
	//Check challenge signatures
	msg, e := challenge.cs.MarshalBinary()
	if e != nil {
		return nil, nil, fmt.Errorf("Error in challenge conversion: %s", e)
	}
	for _, sig := range challenge.sigs {
		e = ECDSAVerify(context.G.Y[sig.index], msg, sig.sig)
		if e != nil {
			return nil, nil, fmt.Errorf("%s", e)
		}
	}

	//Generates the c array
	var ctemp []abstract.Scalar
	for _, temp := range *w {
		ctemp = append(ctemp, temp)
	}
	c = &ctemp
	sum := suite.Scalar().Zero()
	for _, i := range *w {
		sum = suite.Scalar().Add(sum, i)
	}
	(*c)[client.index] = suite.Scalar().Sub(challenge.cs, sum)

	//Generates the responses
	var rtemp []abstract.Scalar
	for _, temp := range *v {
		rtemp = append(rtemp, temp)
	}
	r = &rtemp
	a := suite.Scalar().Mul((*c)[client.index], client.private)
	(*r)[2*client.index] = suite.Scalar().Sub((*v)[2*client.index], a)

	b := suite.Scalar().Mul((*c)[client.index], s)
	(*r)[2*client.index+1] = suite.Scalar().Sub((*v)[2*client.index+1], b)

	return c, r, nil
}

/*verifyClientProof checks the validity of a client's proof*/
func verifyClientProof(msg ClientMessage) bool {
	check := ValidateClientMessage(&msg)
	if !check {
		return false
	}

	n := len(msg.context.G.X)

	//Check the commitments
	for i := 0; i < n; i++ {
		a := suite.Point().Mul(msg.context.G.X[i], msg.proof.c[i])
		b := suite.Point().Mul(nil, msg.proof.r[2*i])
		ti0 := suite.Point().Add(a, b)
		if !ti0.Equal(msg.proof.t[3*i]) {
			return false
		}

		c := suite.Point().Mul(msg.sArray[len(msg.sArray)-1], msg.proof.c[i])
		d := suite.Point().Mul(nil, msg.proof.r[2*i+1])
		ti10 := suite.Point().Add(c, d)
		if !ti10.Equal(msg.proof.t[3*i+1]) {
			return false
		}

		e := suite.Point().Mul(msg.t0, msg.proof.c[i])
		f := suite.Point().Mul(msg.context.H[i], msg.proof.r[2*i+1])
		ti11 := suite.Point().Add(e, f)
		if !ti11.Equal(msg.proof.t[3*i+2]) {
			return false
		}
	}

	//Check the challenge
	cs := suite.Scalar().Zero()
	for _, ci := range msg.proof.c {
		cs = suite.Scalar().Add(cs, ci)
	}
	if !cs.Equal(msg.proof.cs) {
		return false
	}

	return true
}

//AssembleMessage is used to build a Client Message from its various elemnts
func (client *Client) AssembleMessage(context *ContextEd25519, S *[]abstract.Point, T0 abstract.Point, challenge *Challenge, t *[]abstract.Point, c, r *[]abstract.Scalar) (msg *ClientMessage) {
	//Input checks
	if context == nil || S == nil || T0 == nil || challenge == nil || t == nil || c == nil || r == nil {
		return nil
	}
	if len(*S) == 0 || len(*t) == 0 || len(*c) == 0 || len(*r) == 0 {
		return nil
	}

	proof := ClientProof{cs: challenge.cs, t: *t, c: *c, r: *r}
	return &ClientMessage{context: *context, t0: T0, sArray: *S, proof: proof}
}

//GetFinalLinkageTag checks the server's signatures and proofs
//It outputs the final linkage tag of the client
func (client *Client) GetFinalLinkageTag(context *ContextEd25519, msg *ServerMessage) (Tf abstract.Point, err error) {
	//Input checks
	if context == nil || msg == nil {
		return nil, fmt.Errorf("Invalid inputs")
	}

	data, e := msg.request.ToBytes()
	if e != nil {
		return nil, fmt.Errorf("Error in request: %s", e)
	}
	for i := range msg.proofs {
		//Signature check
		temp, err := msg.tags[i].MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("Error in tags: %s", err)
		}
		data = append(data, temp...)

		temp, err = msg.proofs[i].ToBytes()
		if err != nil {
			return nil, fmt.Errorf("Error in proofs: %s", err)
		}
		data = append(data, temp...)

		data = append(data, []byte(strconv.Itoa(msg.indexes[i]))...)

		err = ECDSAVerify(context.G.Y[msg.sigs[i].index], data, msg.sigs[i].sig)
		if err != nil {
			return nil, fmt.Errorf("Error in signature: "+strconv.Itoa(i)+"\n%s", err)
		}

		var valid bool
		p := msg.proofs[i]
		if p.r2 == nil {
			valid = verifyMisbehavingProof(context, i, &p, msg.request.sArray[0])
		} else {
			valid = verifyServerProof(context, i, msg)
		}
		if !valid {
			return nil, fmt.Errorf("Invalid server proof")
		}
	}

	return msg.tags[len(msg.tags)-1], nil
}

/*ValidateClientMessage is an utility function to validate that a client message is correclty formed*/
func ValidateClientMessage(msg *ClientMessage) bool {
	//Number of clients
	i := len(msg.context.G.X)
	//Number of servers
	j := len(msg.context.G.Y)
	//A commitment for each server exists and the second element is the generator S=(Z,g,S1,..,Sj)
	if len(msg.sArray) != j+2 {
		return false
	}
	if !msg.sArray[1].Equal(suite.Point().Mul(nil, suite.Scalar().One())) {
		return false
	}
	//T0 not empty
	if msg.t0 == nil {
		return false
	}
	//Proof fields have the correct size
	if len(msg.proof.c) != i || len(msg.proof.r) != 2*i || len(msg.proof.t) != 3*i || msg.proof.cs == nil {
		return false
	}
	return true
}

/*ToBytes is a helper function used to convert a ClientMessage into []byte to be used in signatures*/
func (msg *ClientMessage) ToBytes() (data []byte, err error) {
	data, e := msg.context.ToBytes()
	if e != nil {
		return nil, fmt.Errorf("Error in context: %s", e)
	}

	temp, e := PointArrayToBytes(&msg.sArray)
	if e != nil {
		return nil, fmt.Errorf("Error in S: %s", e)
	}
	data = append(data, temp...)

	temp, e = msg.t0.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("Error in T0: %s", e)
	}
	data = append(data, temp...)

	temp, e = msg.proof.ToBytes()
	if e != nil {
		return nil, fmt.Errorf("Error in proof: %s", e)
	}
	data = append(data, temp...)

	return data, nil
}

/*ToBytes is a helper function used to convert a ClientProof into []byte to be used in signatures*/
func (proof *ClientProof) ToBytes() (data []byte, err error) {
	data, e := proof.cs.MarshalBinary()
	if e != nil {
		return nil, fmt.Errorf("Error in cs: %s", e)
	}

	temp, e := PointArrayToBytes(&proof.t)
	if e != nil {
		return nil, fmt.Errorf("Error in t: %s", e)
	}
	data = append(data, temp...)

	temp, e = ScalarArrayToBytes(&proof.c)
	if e != nil {
		return nil, fmt.Errorf("Error in c: %s", e)
	}
	data = append(data, temp...)

	temp, e = ScalarArrayToBytes(&proof.r)
	if e != nil {
		return nil, fmt.Errorf("Error in r: %s", e)
	}
	data = append(data, temp...)

	return data, nil
}
