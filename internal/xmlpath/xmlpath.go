package xmlpath

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type Document struct {
	nodes []node
	root  *Element
}

type Element struct {
	start    xml.StartElement
	children []node
}

type nodeKind int

const (
	nodeElement nodeKind = iota
	nodeToken
)

type node struct {
	kind    nodeKind
	element *Element
	token   xml.Token
}

func Parse(data []byte) (*Document, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	doc := &Document{}
	namespaces := map[string]string{}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if doc.root != nil {
				return nil, fmt.Errorf("multiple root elements found")
			}
			element, err := parseElement(decoder, t, namespaces)
			if err != nil {
				return nil, err
			}
			doc.root = element
			doc.nodes = append(doc.nodes, elementNode(element))
		case xml.EndElement:
			return nil, fmt.Errorf("unexpected end element %q", t.Name.Local)
		default:
			if cloned, ok := cloneToken(t); ok {
				doc.nodes = append(doc.nodes, tokenNode(cloned))
			}
		}
	}

	if doc.root == nil {
		return nil, fmt.Errorf("root element not found")
	}

	return doc, nil
}

func parseElement(decoder *xml.Decoder, start xml.StartElement, namespaces map[string]string) (*Element, error) {
	start, namespaces = normalizeStartElement(start, namespaces)
	element := &Element{start: start}

	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			child, err := parseElement(decoder, t, namespaces)
			if err != nil {
				return nil, err
			}
			element.children = append(element.children, elementNode(child))
		case xml.EndElement:
			return element, nil
		default:
			if cloned, ok := cloneToken(t); ok {
				element.children = append(element.children, tokenNode(cloned))
			}
		}
	}
}

func (d *Document) Root() *Element {
	return d.root
}

func (d *Document) MarshalIndent(prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	if prefix != "" || indent != "" {
		encoder.Indent(prefix, indent)
	}

	for _, node := range d.nodes {
		if err := encodeNode(encoder, node); err != nil {
			return nil, err
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (e *Element) Name() string {
	return e.start.Name.Local
}

func (e *Element) Attr(name string) (string, bool) {
	for _, attr := range e.start.Attr {
		if nameMatches(attr.Name.Local, name) {
			return attr.Value, true
		}
	}
	return "", false
}

func (e *Element) SetAttr(name, value string) {
	for index, attr := range e.start.Attr {
		if nameMatches(attr.Name.Local, name) {
			e.start.Attr[index].Value = value
			return
		}
	}
	e.start.Attr = append(e.start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func (e *Element) Text() string {
	var text strings.Builder
	for _, child := range e.children {
		if child.kind != nodeToken {
			continue
		}
		if charData, ok := child.token.(xml.CharData); ok {
			text.Write([]byte(charData))
		}
	}
	return text.String()
}

func (e *Element) SetText(value string) {
	e.children = []node{tokenNode(xml.CharData([]byte(value)))}
}

func (e *Element) Find(path string) *Element {
	matches := e.FindAll(path)
	if len(matches) == 0 {
		return nil
	}
	return matches[0]
}

func (e *Element) FindAll(path string) []*Element {
	parts := splitPath(path)
	if len(parts) == 0 {
		return []*Element{e}
	}
	if nameMatches(e.Name(), parts[0]) {
		parts = parts[1:]
	}
	return e.findAll(parts)
}

func (e *Element) EnsurePath(path string) *Element {
	parts := splitPath(path)
	if len(parts) == 0 {
		return e
	}
	if nameMatches(e.Name(), parts[0]) {
		parts = parts[1:]
	}

	current := e
	for _, part := range parts {
		child := current.findDirect(part)
		if child == nil {
			child = current.AppendChild(part)
		}
		current = child
	}

	return current
}

func (e *Element) AppendChild(name string) *Element {
	child := &Element{start: xml.StartElement{Name: xml.Name{Local: name}}}
	e.children = append(e.children, elementNode(child))
	return child
}

func (e *Element) AppendChildInOrder(name string, order []string) *Element {
	child := &Element{start: xml.StartElement{Name: xml.Name{Local: name}}}
	index := e.insertIndex(name, order)
	e.children = append(e.children, node{})
	copy(e.children[index+1:], e.children[index:])
	e.children[index] = elementNode(child)
	return child
}

func (e *Element) RemoveAll(path string) int {
	parts := splitPath(path)
	if len(parts) == 0 {
		return 0
	}
	if nameMatches(e.Name(), parts[0]) {
		parts = parts[1:]
	}
	return e.removeAll(parts)
}

func (e *Element) findAll(parts []string) []*Element {
	if len(parts) == 0 {
		return []*Element{e}
	}

	var matches []*Element
	for _, child := range e.elementChildren() {
		if !segmentMatches(child.Name(), parts[0]) {
			continue
		}
		matches = append(matches, child.findAll(parts[1:])...)
	}
	return matches
}

func (e *Element) findDirect(name string) *Element {
	for _, child := range e.elementChildren() {
		if segmentMatches(child.Name(), name) {
			return child
		}
	}
	return nil
}

func (e *Element) elementChildren() []*Element {
	children := make([]*Element, 0)
	for _, child := range e.children {
		if child.kind == nodeElement {
			children = append(children, child.element)
		}
	}
	return children
}

func (e *Element) removeAll(parts []string) int {
	if len(parts) == 0 {
		return 0
	}

	if len(parts) == 1 {
		removed := 0
		filtered := e.children[:0]
		for _, child := range e.children {
			if child.kind == nodeElement && segmentMatches(child.element.Name(), parts[0]) {
				removed++
				continue
			}
			filtered = append(filtered, child)
		}
		e.children = filtered
		return removed
	}

	removed := 0
	for _, child := range e.elementChildren() {
		if segmentMatches(child.Name(), parts[0]) {
			removed += child.removeAll(parts[1:])
		}
	}
	return removed
}

func (e *Element) insertIndex(name string, order []string) int {
	targetRank, hasTargetRank := orderRank(name, order)
	if !hasTargetRank {
		return len(e.children)
	}

	insert := len(e.children)
	afterLowerOrEqual := -1
	firstUnknownAfterLower := -1
	for index, child := range e.children {
		if child.kind != nodeElement {
			continue
		}

		childRank, hasChildRank := orderRank(child.element.Name(), order)
		if !hasChildRank {
			if afterLowerOrEqual >= 0 && firstUnknownAfterLower == -1 {
				firstUnknownAfterLower = index
			}
			continue
		}

		if childRank > targetRank {
			insert = index
			break
		}

		afterLowerOrEqual = index
		firstUnknownAfterLower = -1
	}

	if insert == len(e.children) && firstUnknownAfterLower != -1 {
		return firstUnknownAfterLower
	}
	if insert == len(e.children) && afterLowerOrEqual != -1 {
		return afterLowerOrEqual + 1
	}

	return insert
}

func encodeNode(encoder *xml.Encoder, node node) error {
	if node.kind == nodeElement {
		return encodeElement(encoder, node.element)
	}
	return encoder.EncodeToken(node.token)
}

func encodeElement(encoder *xml.Encoder, element *Element) error {
	if err := encoder.EncodeToken(element.start); err != nil {
		return err
	}

	for _, child := range element.children {
		if err := encodeNode(encoder, child); err != nil {
			return err
		}
	}

	return encoder.EncodeToken(xml.EndElement{Name: element.start.Name})
}

func elementNode(element *Element) node {
	return node{kind: nodeElement, element: element}
}

func tokenNode(tok xml.Token) node {
	return node{kind: nodeToken, token: tok}
}

func splitPath(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	parts := raw[:0]
	for _, part := range raw {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func segmentMatches(name, segment string) bool {
	return segment == "*" || nameMatches(name, segment)
}

func nameMatches(name, expected string) bool {
	return name == expected || localName(name) == expected
}

func localName(name string) string {
	if index := strings.IndexByte(name, ':'); index != -1 {
		return name[index+1:]
	}
	return name
}

func orderRank(name string, order []string) (int, bool) {
	for index, candidate := range order {
		if nameMatches(name, candidate) {
			return index, true
		}
	}
	return 0, false
}

func normalizeStartElement(start xml.StartElement, inherited map[string]string) (xml.StartElement, map[string]string) {
	namespaces := cloneNamespaces(inherited)
	for _, attr := range start.Attr {
		switch {
		case attr.Name.Space == "xmlns":
			namespaces[attr.Value] = attr.Name.Local
		case attr.Name.Space == "" && attr.Name.Local == "xmlns":
			namespaces[attr.Value] = ""
		}
	}

	start.Name = nameForMarshal(start.Name, namespaces)
	for index, attr := range start.Attr {
		attr.Name = attrNameForMarshal(attr.Name, namespaces)
		start.Attr[index] = attr
	}

	return start, namespaces
}

func nameForMarshal(name xml.Name, namespaces map[string]string) xml.Name {
	if name.Space == "" {
		return name
	}
	if prefix, ok := namespaces[name.Space]; ok && prefix != "" {
		return xml.Name{Local: prefix + ":" + name.Local}
	}
	return xml.Name{Local: name.Local}
}

func attrNameForMarshal(name xml.Name, namespaces map[string]string) xml.Name {
	if name.Space == "xmlns" {
		return xml.Name{Local: "xmlns:" + name.Local}
	}
	if name.Space == "" {
		return name
	}
	if prefix, ok := namespaces[name.Space]; ok && prefix != "" {
		return xml.Name{Local: prefix + ":" + name.Local}
	}
	return name
}

func cloneNamespaces(namespaces map[string]string) map[string]string {
	cloned := make(map[string]string, len(namespaces))
	for uri, prefix := range namespaces {
		cloned[uri] = prefix
	}
	return cloned
}

func cloneToken(tok xml.Token) (xml.Token, bool) {
	switch t := tok.(type) {
	case xml.CharData:
		return xml.CharData(append([]byte(nil), t...)), true
	case xml.Comment:
		return xml.Comment(append([]byte(nil), t...)), true
	case xml.ProcInst:
		if t.Target == "xml" {
			return nil, false
		}
		return xml.ProcInst{Target: t.Target, Inst: append([]byte(nil), t.Inst...)}, true
	case xml.Directive:
		return xml.Directive(append([]byte(nil), t...)), true
	default:
		return tok, true
	}
}
