package hyperledger

import "fmt"

var fabric Fabric

func WriteTrans(key string, value string) string {
	fmt.Println("Client - Send transaction to Peer")
	rwset1, rwset2 := fabric.WriteTransaction(key, value, fabric.MSP_org1)
	if rwset1.msp == fabric.MSP_peer1 && rwset2.msp == fabric.MSP_peer2 {
		fmt.Println("Client - Endorement {client_msp: auth, key: key, value: value} rwset1 :", rwset1, "/ rwset2:", rwset2)
		msps := []string{rwset1.msp, rwset2.msp}
		rwset := RWSet{key: key, value: value, peers_msp: msps}
		fmt.Println("Client - Send RWSet to Orderer")
		fabric.SendToOrderer(rwset)
		return "ok"
	}

	return "failed"
}

func GetTrans(key string) string {
	fmt.Println("Client - Request read transaction")
	rwset := fabric.ReadTransaction(key, fabric.MSP_org1)
	return rwset
}

func StartFabric() {
	fmt.Println("Start - Fabric prototype")
	fabric = Fabric{}
	fabric.Start()
}
