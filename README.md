Installation:

    go get github.com/chmduquesne/rollinghash

Usage:

    package main
    
    import (
    	"fmt"
    	"github.com/chmduquesne/rollinghash/adler32"
    	"log"
    )
    
    func main() {
    	s := []byte("The brown fox jumps over the lazy dog")
    	hash := adler32.New()
    
    	// window len
    	n := 16
    
    	// Load the window
    	hash.Write(s[0:n])
    
    	// Roll it
    	for i := n; i < len(s); i++ {
    
    		err := hash.Roll(s[i])
    		if err != nil {
    			log.Fatal(err)
    		}
    
    		sum := hash.Sum32()
    
    		fmt.Printf("The adler32sum of %v is %x\n", s[i+1-n : i+1], sum)
    	}
    }
