package gecgosio

type Broadcast map[string]*Peer

func (r Broadcast) Emit(e string, msg ...string) {
	for _, peer := range r {
		peer.Emit(e, msg...)
	}
}
