package utils

import "fmt"

func Confirm() bool {
	fmt.Printf("Continue? (yes/no): ")
	var input string
	fmt.Scanln(&input)
	return input == "yes"
}
