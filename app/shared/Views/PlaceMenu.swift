import SwiftUI

/// PlaceMenu is the menu of where a new todo can go: the todo lists there are, each with its
/// sections, and New Section…, which asks for a name; nt makes the section when the todo is
/// added (docs/design.md §4, Todos). reset, when given, is a first row that takes the choice
/// back (choose(nil)).
struct PlaceMenu<Label: View>: View {
    let places: [TodoPlace]
    let base: String // the folder whose todo.md a new section goes to; "" is the root's
    var reset: String?
    let choose: (TodoPlace?) -> Void
    @ViewBuilder let label: () -> Label
    @State private var naming = false
    @State private var name = ""

    var body: some View {
        Menu {
            if let reset {
                Button(reset) { choose(nil) }
                Divider()
            }
            ForEach(places, id: \.self) { p in
                Button(p.name) { choose(p) }
            }
            Divider()
            Button("New Section…") {
                name = ""
                naming = true
            }
        } label: {
            label()
        }
        .alert("New section in \(TodoPlace(folder: base).file)", isPresented: $naming) {
            TextField("Name", text: $name)
            Button("Cancel", role: .cancel) {}
            Button("Use") {
                let n = name.split(separator: " ").joined(separator: " ")
                if !n.isEmpty { choose(TodoPlace(folder: base, section: n, isNew: true)) }
            }
        } message: {
            Text("The section is made when the todo is added.")
        }
    }
}
