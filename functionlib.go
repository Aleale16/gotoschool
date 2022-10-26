package main

import "fmt"

func Sum(x ...int) (res int) {
    for _, v := range x {
        res += v
    }
    return
} 

func main() {

	sum := Sum(2, 3, 5, 1, 2, 57) 
	fmt.Println(sum)
}