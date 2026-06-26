package banner

import (
	"fmt"
)

// prints the version message
const version = "v0.0.5"

func PrintVersion() {
	fmt.Printf("Current xssrecon version %s\n", version)
}

// Prints the Colorful banner
func PrintBanner() {
	banner := `
   _  __ _____ _____ _____ ___   _____ ____   ____ 
  | |/_// ___// ___// ___// _ \ / ___// __ \ / __ \
 _>  < (__  )(__  )/ /   /  __// /__ / /_/ // / / /
/_/|_|/____//____//_/    \___/ \___/ \____//_/ /_/ 
`
	fmt.Printf("%s\n%55s\n\n", banner, "Current xssrecon version "+version)
}
