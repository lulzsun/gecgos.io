package gecgosio

type Room map[string]*Peer

func (r Room) Emit(e string, msg string) {
	for _, peer := range r {
		peer.Emit(e, msg)
	}
}
