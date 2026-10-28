// nt for Mac: the notes repository in two columns, the folders beside the note, the note in
// live preview. One window; the keys are the terminal browser's, with Command. nt's mark in the
// menu bar, and a shortcut that adds a line to today from anywhere; Settings for nt's and the
// app's own.
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
                    // For screenshots: -NTCapture today or todo opens the capture panel, with
                    // -NTQuery typed into it.
                    if let mode = UserDefaults.standard.string(forKey: "NTCapture") {
                        app.capture.open(mode == "todo" ? .todo : .today)
                        app.capture.text = UserDefaults.standard.string(forKey: "NTQuery") ?? ""
                    }
                }
                .frame(minWidth: 720, minHeight: 480)
        }
        .defaultSize(width: 1180, height: 820)
        .commands {
            CommandGroup(after: .newItem) {
                Button("Add to Today…") { app.capture.open(.today) }
            }
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

        MenuBarExtra {
            MenuBarMenu()
                .environment(workspace)
                .environment(app.capture)
                .environment(app.captureKey)
        } label: {
            MenuBarLabel()
                .environment(workspace)
        }

        Settings {
            SettingsView()
                .environment(workspace)
                .environment(app.captureKey)
        }
    }
}

/// AppDelegate holds the workspace, the capture panel and its shortcut, and saves the note when
/// the person leaves the app or quits. Closing the window leaves nt in the menu bar.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let workspace = Workspace()
    lazy var capture = Capture(workspace: workspace)
    private lazy var panel = CapturePanel(capture: capture)
    lazy var captureKey = CaptureKey { [weak self] in self?.capture.open(.today) }

    func applicationDidFinishLaunching(_ notification: Notification) {
        _ = panel
        captureKey.start()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationDidBecomeActive(_ notification: Notification) {
        workspace.active = true
    }

    func applicationDidResignActive(_ notification: Notification) {
        workspace.active = false
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
