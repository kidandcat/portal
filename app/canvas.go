package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

const (
	toolSelect  = "select"
	toolNote    = "note"
	toolTask    = "task"
	toolHeading = "heading"
)

type CanvasView struct {
	app.Compo

	// Project
	projectID   int64
	projectSlug string
	elements    []Element
	loaded      bool

	// Pan/Zoom
	panX, panY float64
	zoom       float64
	isPanning  bool
	panStartX  float64
	panStartY  float64
	mousePanX  float64
	mousePanY  float64

	// Element drag
	isDragging     bool
	dragElementIdx int
	dragOffsetX    float64
	dragOffsetY    float64

	// Editing
	editingIdx int
	editID     string

	// Context menu
	showContextMenu    bool
	contextMenuX       float64
	contextMenuY       float64
	contextMenuCanvasX float64
	contextMenuCanvasY float64

	// Active tool
	activeTool string

	// Selected element
	selectedIdx int
}

func (c *CanvasView) OnInit() {
	c.zoom = 1.0
	c.editingIdx = -1
	c.selectedIdx = -1
	c.activeTool = toolSelect
}

func (c *CanvasView) OnMount(ctx app.Context) {
	c.loadFromURL(ctx)
}

func (c *CanvasView) OnNav(ctx app.Context) {
	c.loadFromURL(ctx)
}

func (c *CanvasView) loadFromURL(ctx app.Context) {
	path := ctx.Page().URL().Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "p" {
		c.projectSlug = parts[1]
	}
	c.loadData(ctx)
}

func (c *CanvasView) loadData(ctx app.Context) {
	ctx.Async(func() {
		url := "/api/projects"
		if c.projectSlug != "" {
			url = "/api/projects?slug=" + c.projectSlug
		}

		resp, err := http.Get(url)
		if err != nil {
			app.Log("error loading projects:", err)
			return
		}
		defer resp.Body.Close()

		var projects []Project
		json.NewDecoder(resp.Body).Decode(&projects)

		if len(projects) == 0 {
			body, _ := json.Marshal(map[string]string{"name": "My Project", "slug": "default"})
			resp2, err := http.Post("/api/projects", "application/json", bytes.NewReader(body))
			if err != nil {
				app.Log("error creating project:", err)
				return
			}
			defer resp2.Body.Close()
			var p Project
			json.NewDecoder(resp2.Body).Decode(&p)
			projects = []Project{p}
		}

		projectID := projects[0].ID
		projectSlug := projects[0].Slug

		resp3, err := http.Get(fmt.Sprintf("/api/elements?project_id=%d", projectID))
		if err != nil {
			app.Log("error loading elements:", err)
			return
		}
		defer resp3.Body.Close()

		var elements []Element
		json.NewDecoder(resp3.Body).Decode(&elements)
		if elements == nil {
			elements = []Element{}
		}

		ctx.Dispatch(func(ctx app.Context) {
			c.projectID = projectID
			c.projectSlug = projectSlug
			c.elements = elements
			c.loaded = true
		})
	})
}

// Coordinate conversion
func (c *CanvasView) screenToCanvas(sx, sy float64) (float64, float64) {
	return (sx - c.panX) / c.zoom, (sy - c.panY) / c.zoom
}

// Wheel zoom
func (c *CanvasView) onWheel(ctx app.Context, e app.Event) {
	e.PreventDefault()
	delta := e.Get("deltaY").Float()
	mx := e.Get("clientX").Float()
	my := e.Get("clientY").Float()

	factor := 1.1
	if delta > 0 {
		factor = 1 / 1.1
	}

	newZoom := c.zoom * factor
	if newZoom < 0.1 {
		newZoom = 0.1
	}
	if newZoom > 5.0 {
		newZoom = 5.0
	}

	c.panX = mx - (mx-c.panX)*newZoom/c.zoom
	c.panY = my - (my-c.panY)*newZoom/c.zoom
	c.zoom = newZoom
}

// Canvas mouse down - start pan
func (c *CanvasView) onCanvasMouseDown(ctx app.Context, e app.Event) {
	button := e.Get("button").Int()

	// Close context menu on any click
	if c.showContextMenu {
		c.showContextMenu = false
		return
	}

	// Middle mouse or Alt+left starts pan
	if button == 1 || (button == 0 && e.Get("altKey").Bool()) {
		e.PreventDefault()
		c.isPanning = true
		c.mousePanX = e.Get("clientX").Float()
		c.mousePanY = e.Get("clientY").Float()
		c.panStartX = c.panX
		c.panStartY = c.panY
		return
	}

	// Left click on background - deselect
	if button == 0 {
		c.selectedIdx = -1
		if c.editingIdx >= 0 {
			c.finishEditing(ctx)
		}
	}
}

// Canvas mouse move
func (c *CanvasView) onCanvasMouseMove(ctx app.Context, e app.Event) {
	mx := e.Get("clientX").Float()
	my := e.Get("clientY").Float()

	if c.isPanning {
		e.PreventDefault()
		c.panX = c.panStartX + (mx - c.mousePanX)
		c.panY = c.panStartY + (my - c.mousePanY)
		return
	}

	if c.isDragging && c.dragElementIdx >= 0 && c.dragElementIdx < len(c.elements) {
		e.PreventDefault()
		cx, cy := c.screenToCanvas(mx, my)
		c.elements[c.dragElementIdx].X = cx - c.dragOffsetX
		c.elements[c.dragElementIdx].Y = cy - c.dragOffsetY
		return
	}
}

// Canvas mouse up
func (c *CanvasView) onCanvasMouseUp(ctx app.Context, e app.Event) {
	if c.isPanning {
		c.isPanning = false
		return
	}

	if c.isDragging {
		idx := c.dragElementIdx
		c.isDragging = false
		c.dragElementIdx = -1
		if idx >= 0 && idx < len(c.elements) {
			c.saveElementPosition(ctx, idx)
		}
		return
	}
}

// Double click - create element
func (c *CanvasView) onCanvasDblClick(ctx app.Context, e app.Event) {
	e.PreventDefault()
	mx := e.Get("clientX").Float()
	my := e.Get("clientY").Float()
	cx, cy := c.screenToCanvas(mx, my)

	typ := "note"
	switch c.activeTool {
	case toolTask:
		typ = "task"
	case toolHeading:
		typ = "heading"
	}

	c.createElement(ctx, typ, cx, cy)
}

// Right click - context menu
func (c *CanvasView) onContextMenu(ctx app.Context, e app.Event) {
	e.PreventDefault()
	mx := e.Get("clientX").Float()
	my := e.Get("clientY").Float()
	cx, cy := c.screenToCanvas(mx, my)

	c.showContextMenu = true
	c.contextMenuX = mx
	c.contextMenuY = my
	c.contextMenuCanvasX = cx
	c.contextMenuCanvasY = cy
}

// Element mouse down - start drag
func (c *CanvasView) onElementMouseDown(ctx app.Context, e app.Event, idx int) {
	e.Call("stopPropagation")
	button := e.Get("button").Int()

	if button == 1 || e.Get("altKey").Bool() {
		c.isPanning = true
		c.mousePanX = e.Get("clientX").Float()
		c.mousePanY = e.Get("clientY").Float()
		c.panStartX = c.panX
		c.panStartY = c.panY
		return
	}

	if button != 0 {
		return
	}

	if c.editingIdx == idx {
		return
	}

	mx := e.Get("clientX").Float()
	my := e.Get("clientY").Float()
	cx, cy := c.screenToCanvas(mx, my)

	el := c.elements[idx]
	c.isDragging = true
	c.dragElementIdx = idx
	c.dragOffsetX = cx - el.X
	c.dragOffsetY = cy - el.Y
	c.selectedIdx = idx
}

// Element double click - edit
func (c *CanvasView) onElementDblClick(ctx app.Context, e app.Event, idx int) {
	e.Call("stopPropagation")
	e.PreventDefault()

	if c.editingIdx >= 0 && c.editingIdx != idx {
		c.finishEditing(ctx)
	}

	c.editingIdx = idx
	c.editID = fmt.Sprintf("editor-%d", c.elements[idx].ID)
	c.selectedIdx = idx
}

// Task checkbox click
func (c *CanvasView) onCheckboxClick(ctx app.Context, e app.Event, idx int) {
	e.Call("stopPropagation")
	e.PreventDefault()

	if idx < 0 || idx >= len(c.elements) {
		return
	}

	c.elements[idx].Completed = !c.elements[idx].Completed
	c.saveElement(ctx, idx)
}

// Finish editing
func (c *CanvasView) finishEditing(ctx app.Context) {
	if c.editingIdx < 0 || c.editingIdx >= len(c.elements) {
		c.editingIdx = -1
		return
	}

	el := app.Window().GetElementByID(c.editID)
	if !el.Truthy() {
		c.editingIdx = -1
		return
	}

	content := el.Get("value").String()
	c.elements[c.editingIdx].Content = content
	idx := c.editingIdx
	c.editingIdx = -1
	c.saveElement(ctx, idx)
}

func (c *CanvasView) onEditorBlur(ctx app.Context, e app.Event) {
	c.finishEditing(ctx)
}

func (c *CanvasView) onEditorKeyDown(ctx app.Context, e app.Event) {
	key := e.Get("key").String()
	if key == "Escape" {
		c.editingIdx = -1
		return
	}
	// Ctrl+Enter or Cmd+Enter to save
	if key == "Enter" && (e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool()) {
		c.finishEditing(ctx)
	}
}

// Create element via API
func (c *CanvasView) createElement(ctx app.Context, typ string, x, y float64) {
	w := 200.0
	h := 60.0
	if typ == "heading" {
		w = 300.0
		h = 40.0
	}

	// Find max z-index
	maxZ := 0
	for _, el := range c.elements {
		if el.ZIndex > maxZ {
			maxZ = el.ZIndex
		}
	}

	newEl := Element{
		ProjectID: c.projectID,
		Type:      typ,
		Content:   "",
		X:         x - w/2,
		Y:         y - h/2,
		Width:     w,
		Height:    h,
		ZIndex:    maxZ + 1,
	}

	// Add locally first
	localIdx := len(c.elements)
	c.elements = append(c.elements, newEl)
	c.editingIdx = localIdx
	c.editID = fmt.Sprintf("editor-new-%d", localIdx)
	c.selectedIdx = localIdx

	ctx.Async(func() {
		body, _ := json.Marshal(map[string]any{
			"project_id": newEl.ProjectID,
			"type":       newEl.Type,
			"content":    newEl.Content,
			"x":          newEl.X,
			"y":          newEl.Y,
			"width":      newEl.Width,
			"height":     newEl.Height,
			"z_index":    newEl.ZIndex,
		})

		resp, err := http.Post("/api/elements", "application/json", bytes.NewReader(body))
		if err != nil {
			app.Log("error creating element:", err)
			return
		}
		defer resp.Body.Close()

		var created Element
		json.NewDecoder(resp.Body).Decode(&created)

		ctx.Dispatch(func(ctx app.Context) {
			if localIdx < len(c.elements) {
				c.elements[localIdx].ID = created.ID
				c.editID = fmt.Sprintf("editor-%d", created.ID)
			}
		})
	})
}

// Save element position
func (c *CanvasView) saveElementPosition(ctx app.Context, idx int) {
	if idx < 0 || idx >= len(c.elements) {
		return
	}
	el := c.elements[idx]
	if el.ID == 0 {
		return
	}

	ctx.Async(func() {
		body, _ := json.Marshal(map[string]any{
			"x": el.X,
			"y": el.Y,
		})
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/elements/%d", el.ID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			app.Log("error saving position:", err)
			return
		}
		resp.Body.Close()
	})
}

// Save element (content, completed, etc.)
func (c *CanvasView) saveElement(ctx app.Context, idx int) {
	if idx < 0 || idx >= len(c.elements) {
		return
	}
	el := c.elements[idx]
	if el.ID == 0 {
		return
	}

	ctx.Async(func() {
		body, _ := json.Marshal(map[string]any{
			"type":      el.Type,
			"content":   el.Content,
			"x":         el.X,
			"y":         el.Y,
			"width":     el.Width,
			"height":    el.Height,
			"completed": el.Completed,
			"z_index":   el.ZIndex,
		})
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/elements/%d", el.ID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			app.Log("error saving element:", err)
			return
		}
		resp.Body.Close()
	})
}

// Delete selected element
func (c *CanvasView) deleteSelected(ctx app.Context) {
	if c.selectedIdx < 0 || c.selectedIdx >= len(c.elements) {
		return
	}
	el := c.elements[c.selectedIdx]
	idx := c.selectedIdx

	c.elements = append(c.elements[:idx], c.elements[idx+1:]...)
	c.selectedIdx = -1
	c.editingIdx = -1

	if el.ID == 0 {
		return
	}

	ctx.Async(func() {
		req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/elements/%d", el.ID), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			app.Log("error deleting element:", err)
			return
		}
		resp.Body.Close()
	})
}

// Context menu action
func (c *CanvasView) onContextMenuAction(ctx app.Context, e app.Event, typ string) {
	e.Call("stopPropagation")
	e.PreventDefault()
	cx := c.contextMenuCanvasX
	cy := c.contextMenuCanvasY
	c.showContextMenu = false
	c.createElement(ctx, typ, cx, cy)
}

// Keyboard handler
func (c *CanvasView) onKeyDown(ctx app.Context, e app.Event) {
	if c.editingIdx >= 0 {
		return
	}

	key := e.Get("key").String()
	switch key {
	case "Delete", "Backspace":
		c.deleteSelected(ctx)
	case "Escape":
		c.selectedIdx = -1
		c.showContextMenu = false
	}
}

// Set active tool
func (c *CanvasView) setTool(ctx app.Context, e app.Event, tool string) {
	e.Call("stopPropagation")
	e.PreventDefault()
	c.activeTool = tool
}

// Render
func (c *CanvasView) Render() app.UI {
	if !c.loaded {
		return app.Div().Class("loading-overlay").Body(
			app.Div().Class("loading-spinner"),
		)
	}

	gridSize := 20 * c.zoom
	bgPosX := math.Mod(c.panX, gridSize)
	bgPosY := math.Mod(c.panY, gridSize)

	containerClass := "canvas-container"
	if c.isPanning {
		containerClass += " panning"
	}

	return app.Div().
		Class(containerClass).
		TabIndex(-1).
		OnWheel(c.onWheel).
		OnMouseDown(c.onCanvasMouseDown).
		OnMouseMove(c.onCanvasMouseMove).
		OnMouseUp(c.onCanvasMouseUp).
		OnDblClick(c.onCanvasDblClick).
		OnContextMenu(c.onContextMenu).
		OnKeyDown(c.onKeyDown).
		Style("background-size", fmt.Sprintf("%.1fpx %.1fpx", gridSize, gridSize)).
		Style("background-position", fmt.Sprintf("%.1fpx %.1fpx", bgPosX, bgPosY)).
		Body(
			// Canvas transform layer
			app.Div().
				Class("canvas-transform").
				Style("transform", fmt.Sprintf("translate(%.1fpx, %.1fpx) scale(%.4f)", c.panX, c.panY, c.zoom)).
				Body(
					app.Range(c.elements).Slice(func(i int) app.UI {
						return c.renderElement(i)
					}),
				),

			// Toolbar
			c.renderToolbar(),

			// Zoom indicator
			app.Div().
				Class("zoom-indicator").
				Text(fmt.Sprintf("%.0f%%", c.zoom*100)),

			// Context menu
			app.If(c.showContextMenu, func() app.UI {
				return c.renderContextMenu()
			}),
		)
}

func (c *CanvasView) renderElement(idx int) app.UI {
	if idx >= len(c.elements) {
		return app.Div()
	}
	el := c.elements[idx]

	switch el.Type {
	case "task":
		return c.renderTask(idx, el)
	case "heading":
		return c.renderHeading(idx, el)
	default:
		return c.renderNote(idx, el)
	}
}

func (c *CanvasView) renderNote(idx int, el Element) app.UI {
	classes := "element element-note"
	if c.isDragging && c.dragElementIdx == idx {
		classes += " dragging"
	}
	if c.selectedIdx == idx {
		classes += " selected"
	}
	if c.editingIdx == idx {
		classes += " editing"
	}

	editorID := fmt.Sprintf("editor-%d", el.ID)
	if el.ID == 0 {
		editorID = fmt.Sprintf("editor-new-%d", idx)
	}

	return app.Div().
		Class(classes).
		Style("left", fmt.Sprintf("%.1fpx", el.X)).
		Style("top", fmt.Sprintf("%.1fpx", el.Y)).
		Style("width", fmt.Sprintf("%.1fpx", el.Width)).
		Style("min-height", fmt.Sprintf("%.1fpx", el.Height)).
		Style("z-index", fmt.Sprintf("%d", el.ZIndex)).
		OnMouseDown(func(ctx app.Context, e app.Event) {
			c.onElementMouseDown(ctx, e, idx)
		}).
		OnDblClick(func(ctx app.Context, e app.Event) {
			c.onElementDblClick(ctx, e, idx)
		}).
		Body(
			app.If(c.editingIdx == idx, func() app.UI {
				return app.Textarea().
					ID(editorID).
					Class("element-editor").
					Text(el.Content).
					AutoFocus(true).
					OnBlur(c.onEditorBlur).
					OnKeyDown(c.onEditorKeyDown).
					OnMouseDown(func(ctx app.Context, e app.Event) {
						e.Call("stopPropagation")
					})
			}).Else(func() app.UI {
				if el.Content == "" {
					return app.Div().Class("element-content placeholder").Text("Double-click to edit...")
				}
				return app.Div().Class("element-content").Text(el.Content)
			}),
		)
}

func (c *CanvasView) renderTask(idx int, el Element) app.UI {
	classes := "element element-task"
	if c.isDragging && c.dragElementIdx == idx {
		classes += " dragging"
	}
	if c.selectedIdx == idx {
		classes += " selected"
	}
	if c.editingIdx == idx {
		classes += " editing"
	}
	if el.Completed {
		classes += " completed"
	}

	checkboxClass := "task-checkbox"
	if el.Completed {
		checkboxClass += " checked"
	}

	editorID := fmt.Sprintf("editor-%d", el.ID)
	if el.ID == 0 {
		editorID = fmt.Sprintf("editor-new-%d", idx)
	}

	return app.Div().
		Class(classes).
		Style("left", fmt.Sprintf("%.1fpx", el.X)).
		Style("top", fmt.Sprintf("%.1fpx", el.Y)).
		Style("width", fmt.Sprintf("%.1fpx", el.Width)).
		Style("min-height", fmt.Sprintf("%.1fpx", el.Height)).
		Style("z-index", fmt.Sprintf("%d", el.ZIndex)).
		OnMouseDown(func(ctx app.Context, e app.Event) {
			c.onElementMouseDown(ctx, e, idx)
		}).
		OnDblClick(func(ctx app.Context, e app.Event) {
			c.onElementDblClick(ctx, e, idx)
		}).
		Body(
			app.Div().
				Class(checkboxClass).
				OnMouseDown(func(ctx app.Context, e app.Event) {
					c.onCheckboxClick(ctx, e, idx)
				}),
			app.If(c.editingIdx == idx, func() app.UI {
				return app.Textarea().
					ID(editorID).
					Class("element-editor").
					Text(el.Content).
					AutoFocus(true).
					OnBlur(c.onEditorBlur).
					OnKeyDown(c.onEditorKeyDown).
					OnMouseDown(func(ctx app.Context, e app.Event) {
						e.Call("stopPropagation")
					})
			}).Else(func() app.UI {
				if el.Content == "" {
					return app.Div().Class("element-content placeholder").Text("Double-click to edit...")
				}
				return app.Div().Class("element-content").Text(el.Content)
			}),
		)
}

func (c *CanvasView) renderHeading(idx int, el Element) app.UI {
	classes := "element element-heading"
	if c.isDragging && c.dragElementIdx == idx {
		classes += " dragging"
	}
	if c.selectedIdx == idx {
		classes += " selected"
	}
	if c.editingIdx == idx {
		classes += " editing"
	}

	editorID := fmt.Sprintf("editor-%d", el.ID)
	if el.ID == 0 {
		editorID = fmt.Sprintf("editor-new-%d", idx)
	}

	return app.Div().
		Class(classes).
		Style("left", fmt.Sprintf("%.1fpx", el.X)).
		Style("top", fmt.Sprintf("%.1fpx", el.Y)).
		Style("width", fmt.Sprintf("%.1fpx", el.Width)).
		Style("min-height", fmt.Sprintf("%.1fpx", el.Height)).
		Style("z-index", fmt.Sprintf("%d", el.ZIndex)).
		OnMouseDown(func(ctx app.Context, e app.Event) {
			c.onElementMouseDown(ctx, e, idx)
		}).
		OnDblClick(func(ctx app.Context, e app.Event) {
			c.onElementDblClick(ctx, e, idx)
		}).
		Body(
			app.If(c.editingIdx == idx, func() app.UI {
				return app.Textarea().
					ID(editorID).
					Class("element-editor").
					Text(el.Content).
					AutoFocus(true).
					OnBlur(c.onEditorBlur).
					OnKeyDown(c.onEditorKeyDown).
					OnMouseDown(func(ctx app.Context, e app.Event) {
						e.Call("stopPropagation")
					})
			}).Else(func() app.UI {
				if el.Content == "" {
					return app.Div().Class("element-content placeholder").Text("Double-click to edit...")
				}
				return app.Div().Class("element-content").Text(el.Content)
			}),
		)
}

func (c *CanvasView) renderToolbar() app.UI {
	btn := func(icon, tool, title string) app.UI {
		cls := "toolbar-btn"
		if c.activeTool == tool {
			cls += " active"
		}
		return app.Button().
			Class(cls).
			Title(title).
			Text(icon).
			OnMouseDown(func(ctx app.Context, e app.Event) {
				c.setTool(ctx, e, tool)
			})
	}

	return app.Div().Class("toolbar").Body(
		btn("\u25E6", toolSelect, "Select (V)"),
		btn("\u25A1", toolNote, "Note (N)"),
		btn("\u2611", toolTask, "Task (T)"),
		btn("H", toolHeading, "Heading (H)"),
		app.Div().Class("toolbar-divider"),
		app.Button().
			Class("toolbar-btn").
			Title("Zoom to fit").
			Text("\u2922").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				e.Call("stopPropagation")
				e.PreventDefault()
				c.zoom = 1.0
				c.panX = 0
				c.panY = 0
			}),
	)
}

func (c *CanvasView) renderContextMenu() app.UI {
	item := func(icon, label, typ string) app.UI {
		return app.Button().
			Class("context-menu-item").
			OnMouseDown(func(ctx app.Context, e app.Event) {
				c.onContextMenuAction(ctx, e, typ)
			}).
			Body(
				app.Span().Class("icon").Text(icon),
				app.Span().Text(label),
			)
	}

	return app.Div().
		Class("context-menu").
		Style("left", fmt.Sprintf("%.0fpx", c.contextMenuX)).
		Style("top", fmt.Sprintf("%.0fpx", c.contextMenuY)).
		OnMouseDown(func(ctx app.Context, e app.Event) {
			e.Call("stopPropagation")
		}).
		Body(
			item("\u25A1", "Note", "note"),
			item("\u2611", "Task", "task"),
			item("H", "Heading", "heading"),
		)
}
