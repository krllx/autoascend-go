package sense

import (
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// ---- #call

// callItemF is call_item(item, name): returns true.
type callItemF struct {
	Item *world.Item
	Name string
	PC   int
}

func CallItem(item *world.Item, name string) Frame { return &callItemF{Item: item, Name: name} }

func (s *callItemF) Resume(c *Ctx) Cmd {
	inv := c.W.Inventory
	if s.PC == 0 {
		Assert(inv.InAllItems(s.Item), s.Item)
		letter := inv.Items.GetLetter(s.Item)
		s.PC = 1
		d := c.W.Term.Dialog(A_Command_CALL, c.W.Term.RawText("i"+letter+"#"+s.Name+"\r"))
		return call(Atom(d))
	}
	return ret(true)
}
