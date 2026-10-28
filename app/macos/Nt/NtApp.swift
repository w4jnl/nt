// nt for Mac: the notes repository in two columns, the folders beside the note, the note in
// live preview. One window; the keys are the terminal browser's, with Command.
import SwiftUI

@main
struct NtApp: App {
    @State private var workspace = Workspace()

    var body: some Scene {
        Window("nt", id: "main") {
            ContentView()
                .environment(workspace)
                .task {
                    // For screenshots: -NTAppearance dark or light, else the system's.
                    switch UserDefaults.standard.string(forKey: "NTAppearance") {
                    case "dark": NSApp.appearance = NSAppearance(named: .darkAqua)
                    case "light": NSApp.appearance = NSAppearance(named: .aqua)
                    default: break
                    }
                    await workspace.start()
                }
                .frame(minWidth: 720, minHeight: 480)
        }
        .defaultSize(width: 1180, height: 820)
        .commands {
            CommandGroup(before: .sidebar) {
                Button(workspace.sidebar ? "Hide Sidebar" : "Show Sidebar") { workspace.sidebar.toggle() }
                    .keyboardShortcut("s", modifiers: [.command, .control])
            }
            CommandGroup(replacing: .newItem) {
                Button("Today") { Task { await workspace.openToday() } }
                    .keyboardShortcut("t")
                Button("Open…") { workspace.palette = .open }
                    .keyboardShortcut("p")
                Button("Search notes…") { workspace.palette = .text }
                    .keyboardShortcut("f", modifiers: [.command, .shift])
                Divider()
                Button("Sync now") { Task { await workspace.syncNow() } }
                    .keyboardShortcut("s", modifiers: [.command, .option])
            }
        }
    }
}
