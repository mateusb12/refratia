package pentacam
import ("context";"os";"testing";"fmt";"image";"refratia/backend/shared/ocr")
func TestTmpA(t *testing.T){
 d,_:=os.ReadFile("/tmp/claude-1000/-home-mateus-Desktop-Javascript-refratia/1fd31442-3487-4858-b33c-f8d40209f14c/scratchpad/cls/cos450-04.png")
 i,_:=ocr.DecodeImage(d)
 m,_:=readAnchored(context.Background(),map[Screen]int{ScreenRefractive:4},map[int]image.Image{4:i})
 for k,v:=range m{fmt.Println("OK",k,v.Value,v.Hint.Votes,v.Hint.BBox)}
}
