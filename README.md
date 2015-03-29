How to
======

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
    	for i := 1; i < len(s)-n; i++ {
    
    		err := hash.Roll(s[i-1], s[i+n-1])
    		if err != nil {
    			log.Fatal(err)
    		}
    
    		sum := hash.Sum32()
    
    		fmt.Printf("%v has checksum %x\n", s[i:i+n], sum)
    	}
    }

