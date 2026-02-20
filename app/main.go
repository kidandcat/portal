package main

import "github.com/maxence-charriere/go-app/v10/pkg/app"

func main() {
	app.Route("/", func() app.Composer { return &CanvasView{} })
	app.RouteWithRegexp(`^/p/.+$`, func() app.Composer { return &CanvasView{} })
	app.RunWhenOnBrowser()
}
