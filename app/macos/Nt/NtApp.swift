// nt for Mac: the notes repository in two columns, the folders beside the note, the note in
// live preview. One window; the keys are the terminal browser's, with Command.
import SwiftUI

@main
struct NtApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var app
    private var workspace: Workspace { app.workspace }

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
                Toggle("Focus Mode", isOn: Binding(get: { workspace.focusMode }, set: { _ in workspace.toggleFocus() }))
                    .keyboardShortcut("l", modifiers: [.command, .shift])
            }
            CommandGroup(replacing: .newItem) {
                Button("New Note…") { workspace.palette = .new }
                    .keyboardShortcut("n")
                    .disabled(workspace.phase != .ready)
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

/// AppDelegate holds the workspace and saves the note when the person leaves the app or quits.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let workspace = Workspace()

    func applicationDidResignActive(_ notification: Notification) {
        Task { await workspace.settle() }
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard workspace.editor != nil else { return .terminateNow }
        Task {
            let ok = await workspace.finish()
            sender.reply(toApplicationShouldTerminate: ok)
        }
        return .terminateLater
    }
}
