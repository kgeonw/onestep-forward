package hyperledger

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// TRANACTION
type Tranaction struct {
	client_msp string
	key        string
	value      string
}

type _Tranaction struct {
	key   string
	value string
}

// RWSEt
type RWSet struct {
	msp       string
	peers_msp []string
	key       string
	value     string
}

// LEVEL DB
//state key-value storage
var level_mutex = &sync.RWMutex{}

type LevelDB struct {
	m map[string]string
}

func (db *LevelDB) getValue(key string) string {
	level_mutex.RLock()
	defer level_mutex.RUnlock()
	fmt.Println("LevelDB - GetValue Key:", key)
	return db.m[key]
}

func (db *LevelDB) setValue(key string, value string) {
	level_mutex.Lock()
	defer level_mutex.Unlock()
	fmt.Println("LevelDB - SetValue key:", key, "value:", value)
	db.m[key] = value
}

// BLOCK
type Block struct {
	endorsers []string
	Trans     []_Tranaction
}

type _Block struct {
	Index     int
	Timestamp string
	Trans     []_Tranaction
	Hash      string
	PrevHash  string
}

// LEDGER
var ledger_mutex = &sync.Mutex{}

type Ledger struct {
	Blockchain []_Block
	*LevelDB
	time time.Time
}

func (l *Ledger) createGenesisBlock() {
	fmt.Println("Ledger - CreateGenesisBlock")
	t := time.Now()
	genesisBlock := _Block{}
	genesisBlock = _Block{0, t.String(), nil, calculateHash(genesisBlock), ""}
	//spew.Dump(genesisBlock)

	ledger_mutex.Lock()
	l.Blockchain = append(l.Blockchain, genesisBlock)
	ledger_mutex.Unlock()
}

func (l *Ledger) addBlock(block Block) {
	ledger_mutex.Lock()
	prevBlock := l.Blockchain[len(l.Blockchain)-1]
	newBlock := l.generateBlock(prevBlock, block)
	l.Blockchain = append(l.Blockchain, newBlock)
	//spew.Dump(newBlock)
	fmt.Println("Ledger - Add new Block", block)
	ledger_mutex.Unlock()
}

// create a new block using previous block's hash
func (o *Ledger) generateBlock(oldBlock _Block, block Block) _Block {
	var newBlock _Block
	t := time.Now()
	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateHash(newBlock)
	fmt.Println("Ledger - Generate Block", block)
	return newBlock
}

func (l *Ledger) setState(trans _Tranaction) {
	fmt.Println("Ledger - SetState Transaction", trans)
	l.setValue(trans.key, trans.value)
}

func (l *Ledger) getState(trans Tranaction) string {
	fmt.Println("Ledger - GetState Transaction", trans)
	return l.getValue(trans.key)
}

// PEER
type Peer struct {
	peer_type  int // 0 commit only peer, 1 endorse + commit peer
	ledger     *Ledger
	msp        MSP
	addtrans   chan Tranaction
	addblock   chan Block
	endsorseok chan RWSet
	peer_done  chan bool

	fabric *Fabric
}

func (p *Peer) Start() {
	p.addtrans = make(chan Tranaction)
	p.addblock = make(chan Block)
	p.endsorseok = make(chan RWSet)
	p.peer_done = make(chan bool)

	if p.peer_type == 1 {
		go p.endorsing()
	}
	go p.committing()
}

func (p *Peer) addTrans(trans Tranaction) RWSet {
	p.addtrans <- trans
	fmt.Println("Peer - Receive transaction from Client", trans)
	return <-p.endsorseok
}

func (p *Peer) endorsing() {
	go func() {
		for {
			select {
			case trans := <-p.addtrans:
				if trans.client_msp == p.fabric.MSP_org1 {
					//
					//execute chain code !!!
					//
					rwset := RWSet{key: trans.key, value: trans.value, msp: p.msp.id}
					p.endsorseok <- rwset
				}
				fmt.Println("Peer - Excution", trans)
			case <-p.peer_done:
				fmt.Println("Peer - Done")
				return
			}
		}
	}()
}

func (p *Peer) committing() {
	go func() {
		for {
			select {
			case block := <-p.addblock:
				ok := p.validating(block)
				if ok == false {
					continue
				}
				for _, trans := range block.Trans {
					p.ledger.setState(trans)
				}
				p.ledger.addBlock(block)
			case <-p.peer_done:
				return
			}
		}
	}()
}

func (p *Peer) validating(block Block) bool {
	if block.endorsers[0] == p.fabric.MSP_peer1 && block.endorsers[1] == p.fabric.MSP_peer2 {
		fmt.Println("Peer - Validating", block)
		return true
	}
	return false
}

func (p *Peer) getData(key string) string {
	return p.ledger.getValue(key)
}

// ORDERER
type Orderer struct {
	msp          MSP
	addrwset     chan RWSet
	orderer_done chan bool
	committer    []*Peer
	kafka        *Kafka

	fabric *Fabric
}

func (o *Orderer) Start() {
	o.addrwset = make(chan RWSet)
	o.orderer_done = make(chan bool)

	go o.producer()
	go o.consumer()
}

func (o *Orderer) addRWSet(rwset RWSet) {
	o.addrwset <- rwset
}

func (o *Orderer) producer() {
	go func() {
		for {
			select {
			case rwset := <-o.addrwset:
				fmt.Println("Orderer - Send RWSet To Kafka")
				o.kafka.Push(rwset)
			case <-o.orderer_done:
				return
			}
		}
	}()
}

func (o *Orderer) consumer() {
	go func() {
		for {
			rwsets := o.kafka.Pull()
			if rwsets == nil {
				runtime.Gosched()
				continue
			}
			newBlock := o.createBlock(rwsets)
			for _, committer := range o.committer {
				committer.addblock <- newBlock
			}
		}
	}()
}

func (o *Orderer) createBlock(rwsets []RWSet) Block {
	var newBlock Block
	for _, rwset := range rwsets {
		_trans := _Tranaction{key: rwset.key, value: rwset.value}
		newBlock.Trans = append(newBlock.Trans, _trans)
		newBlock.endorsers = append(newBlock.endorsers, rwset.peers_msp[0])
		newBlock.endorsers = append(newBlock.endorsers, rwset.peers_msp[1])
	}
	fmt.Println("Order - Create Block")
	return newBlock
}

//==================================  KAFKA  =================================//
var kafka_mutex = &sync.Mutex{}

type Kafka struct {
	Channel []RWSet
}

func (o *Kafka) Push(rwset RWSet) {
	kafka_mutex.Lock()
	defer kafka_mutex.Unlock()
	fmt.Println("Kafka - Push RWSet")
	o.Channel = append(o.Channel, rwset)
}

func (o *Kafka) Pull() []RWSet {
	kafka_mutex.Lock()
	defer kafka_mutex.Unlock()

	if len(o.Channel) > 0 {
		three := make([]RWSet, 1)
		copy(three, o.Channel[:1])
		o.Channel = append(o.Channel[:0], o.Channel[1:]...)
		fmt.Println("Kafka - Pull RWSet")
		return three
	}
	return nil
}

//==================================  Fabric-CA & MSP =================================//
// MSP
type MSP struct {
	pubKey *ecdsa.PrivateKey
	priKey *ecdsa.PublicKey

	id string
}

func (msp *MSP) validating(id string) bool {
	return msp.id == id
}

type FabricCA struct {
}

//func (ca *FabricCA) getKeyPair(seed string) (*ecdsa.PrivateKey, error) {
//	return crypto.GenerateKey()
//}
//
//func (ca *FabricCA) getID() string {
//	key, err := crypto.GenerateKey()
//	if err != nil {
//		utils.Fatalf("Failed to generate private key: %s", err)
//	}
//	k := hex.EncodeToString(crypto.FromECDSA(key))
//	return k
//}

func (ca *FabricCA) getID() string {
	buff := make([]byte, 10)
	rand.Read(buff)
	str := base64.StdEncoding.EncodeToString(buff)
	return str[:10]
}

//==================================  FABRIC  =================================//
type Fabric struct {
	kafka     *Kafka
	orderer1  *Orderer
	orderer2  *Orderer
	endorser1 *Peer
	endorser2 *Peer
	committer *Peer
	ca        *FabricCA

	roundrobin bool

	MSP_org1     string
	MSP_peer1    string
	MSP_peer2    string
	MSP_peer3    string
	MSP_orderer1 string
	MSP_orderer2 string
}

func (fab *Fabric) Start() {
	// 1. three peer simulator start (two endorsing peer, one committing only peer)

	ledger1 := &Ledger{Blockchain: make([]_Block, 100000), LevelDB: &LevelDB{m: make(map[string]string, 1000000)}}
	ledger2 := &Ledger{Blockchain: make([]_Block, 100000), LevelDB: &LevelDB{m: make(map[string]string, 1000000)}}
	ledger3 := &Ledger{Blockchain: make([]_Block, 100000), LevelDB: &LevelDB{m: make(map[string]string, 1000000)}}

	fab.ca = &FabricCA{}
	fab.MSP_org1 = fab.ca.getID()
	fab.MSP_peer1 = fab.ca.getID()
	fab.endorser1 = &Peer{peer_type: 1, msp: MSP{id: fab.MSP_peer1}, fabric: fab, ledger: ledger1}
	fab.endorser1.Start()
	fab.MSP_peer2 = fab.ca.getID()
	fab.endorser2 = &Peer{peer_type: 1, msp: MSP{id: fab.MSP_peer2}, fabric: fab, ledger: ledger2}
	fab.endorser2.Start()
	fab.MSP_peer3 = fab.ca.getID()
	fab.committer = &Peer{peer_type: 0, msp: MSP{id: fab.MSP_peer3}, fabric: fab, ledger: ledger3}
	fab.committer.Start()

	// 2. kafka simulator start
	fab.kafka = &Kafka{}

	// 3. two orderer simulator start (first is input, second is ordering)
	//fab.MSP_orderer1 = fab.ca.getID()
	//fab.orderer1 = &Orderer{msp: MSP{id: fab.MSP_orderer1},  kafka: fab.kafka, fabric: fab}
	//fab.orderer1.Start()
	fab.MSP_orderer1 = fab.ca.getID()
	committerList := []*Peer{fab.committer} // generally endorsing peer also have committing peer role but excluded to simplify.
	fab.orderer1 = &Orderer{msp: MSP{id: fab.MSP_orderer1}, kafka: fab.kafka, committer: committerList, fabric: fab}
	fab.orderer1.Start()
}

func (fab *Fabric) WriteTransaction(key string, value string, auth string) (RWSet, RWSet) {
	t := Tranaction{client_msp: auth, key: key, value: value}
	rwset1 := fab.endorser1.addTrans(t)
	rwset2 := fab.endorser2.addTrans(t)

	// fmt.Println("OK - Receive To Transaction")
	return rwset1, rwset2
}

func (fab *Fabric) ReadTransaction(key string, auth string) string {
	return fab.committer.getData(key)
}

func (fab *Fabric) SendToOrderer(rwset RWSet) {
	fmt.Println("Orderer - Receive RWSet")
	fab.orderer1.addRWSet(rwset)

	//if fab.roundrobin {
	//	fab.orderer1.addRWSet(rwset)
	//	fab.roundrobin = false
	//} else {
	//	fab.orderer2.addRWSet(rwset)
	//	fab.roundrobin = true
	//}
}

func calculateHash(block _Block) string {
	var trans_concated string
	for _, trans := range block.Trans {
		trans_concated += trans.value
	}
	record := strconv.Itoa(block.Index) + block.Timestamp + trans_concated + block.PrevHash
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}
