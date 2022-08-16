package test

import (
	"fmt"
	"testing"
)

func TestXxx(t *testing.T) {
	type wemixNode struct {
		Name string `json:"name"`
	}

	nodesMap := map[string]*wemixNode{}

	nodesMap["1"] = &wemixNode{Name: "1"}
	nodesMap["2"] = &wemixNode{Name: "2"}

	var nodesSlice []*wemixNode

	for _, i := range nodesMap {
		n := new(wemixNode)
		*n = *i
		nodesSlice = append(nodesSlice, n)
	}
	// fmt.Println(nodesMap)
	// fmt.Println(nodesSlice)

	nodesSlice[0].Name = "11"
	nodesSlice[1].Name = "22"

	// for _, i := range nodesMap {
	// 	fmt.Println(i.Name)
	// }

	// for _, i := range nodesSlice {
	// 	fmt.Println(i.Name)
	// }

	a := "a"
	b := "aa"

	fmt.Println(a < b)

}
