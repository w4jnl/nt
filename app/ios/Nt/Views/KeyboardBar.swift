// KeyboardBar is the bar above the keyboard while a note is written (docs/design.md §9): a task
// box, a heading, a link, a photo, the mic, undo, redo, and done, which lets the keyboard go. It
// takes the place of WebKit's own bar (Previous, Next, Done), which has nothing to do in a note.
import ObjectiveC
import PhotosUI
import SwiftUI
import UIKit

struct KeyboardBar: View {
    @Environment(Workspace.self) private var ws
    @AppStorage(Language.key) private var language = Language.remembered // chosen here, on Talk a note, or in the capture sheet

    var body: some View {
        HStack(spacing: 0) {
            key("Task", "circle") { ws.editor?.task() }
            Button { ws.editor?.heading() } label: {
                Text("#").font(Brand.mono(19, bold: true)).frame(maxWidth: .infinity, minHeight: 44)
            }
            .accessibilityLabel("Heading")
            key("Link", "link") { ws.editor?.link() }
            key("Photo", "photo") { ws.picking = true }
            mic
            key("Undo", "arrow.uturn.backward", enabled: ws.undoable) { ws.editor?.undo() }
            key("Redo", "arrow.uturn.forward", enabled: ws.redoable) { ws.editor?.redo() }
            key("Done writing", "keyboard.chevron.compact.down", tint: Brand.accent) { ws.editor?.done() }
        }
        .foregroundStyle(Brand.ink)
        .frame(height: 48)
        .padding(.horizontal, 6)
        .background(Brand.surface)
        .overlay(alignment: .top) { Rectangle().fill(Brand.line).frame(height: 1) }
    }

    /// mic dictates at the cursor, or stops; held, it chooses the language, whose code shows
    /// under it.
    private var mic: some View {
        let listener = ws.dictation
        return Menu {
            Picker("Language", selection: Binding(get: { language }, set: { l in Task { await ws.dictate(in: l) } })) {
                ForEach(Language.allCases) { Text($0.name).tag($0) }
            }
        } label: {
            VStack(spacing: 2) {
                Group {
                    switch listener.state {
                    case .preparing:
                        ProgressView().controlSize(.small)
                    case .listening:
                        Image(systemName: "mic.fill")
                            .foregroundStyle(Brand.alert)
                            .symbolEffect(.pulse, options: .repeating)
                    default:
                        Image(systemName: "mic").foregroundStyle(Brand.accent)
                    }
                }
                .font(.system(size: 17))
                .frame(height: 20)
                Text(language.short)
                    .font(Brand.mono(9, bold: true))
                    .foregroundStyle(Brand.muted)
            }
            .frame(maxWidth: .infinity, minHeight: 44)
        } primaryAction: {
            Task { await ws.dictate() }
        }
        .accessibilityLabel(listener.busy ? "Stop dictating" : "Dictate at the cursor, in \(language.name)")
    }

    private func key(_ label: String, _ symbol: String, enabled: Bool = true, tint: Color? = nil, _ action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 19))
                .foregroundStyle(enabled ? tint ?? Brand.ink : Brand.faint)
                .frame(maxWidth: .infinity, minHeight: 44)
        }
        .disabled(!enabled)
        .accessibilityLabel(label)
    }

    /// host is the bar as a view for the keyboard to carry, sized as a bar.
    static func host(_ ws: Workspace) -> UIView {
        let host = UIHostingController(rootView: KeyboardBar().environment(ws))
        host.view.backgroundColor = .clear
        let bar = UIView(frame: CGRect(x: 0, y: 0, width: 320, height: 48))
        bar.autoresizingMask = [.flexibleWidth]
        host.view.frame = bar.bounds
        host.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        bar.addSubview(host.view)
        objc_setAssociatedObject(bar, barKey, host, .OBJC_ASSOCIATION_RETAIN_NONATOMIC) // keeps the controller
        return bar
    }
}

nonisolated(unsafe) private let barKey = UnsafeRawPointer(UnsafeMutableRawPointer.allocate(byteCount: 1, alignment: 1))

extension NoteWebView {
    /// setKeyboardBar puts bar above the keyboard. The view that takes the keys is WebKit's own
    /// content view inside the web view, so its class is given a subclass, once, whose
    /// inputAccessoryView is the bar kept on it.
    func setKeyboardBar(_ bar: UIView) {
        guard let content = scrollView.subviews.first(where: { NSStringFromClass(type(of: $0)).hasPrefix("WKContent") }) else { return }
        let base: AnyClass = type(of: content)
        let name = NSStringFromClass(base) + "_NtKeyboardBar"
        var sub: AnyClass? = NSClassFromString(name)
        if sub == nil, let made = objc_allocateClassPair(base, name, 0) {
            let sel = #selector(getter: UIResponder.inputAccessoryView)
            let get: @convention(block) (AnyObject) -> UIView? = { me in objc_getAssociatedObject(me, barKey) as? UIView }
            if let m = class_getInstanceMethod(base, sel) {
                class_addMethod(made, sel, imp_implementationWithBlock(get), method_getTypeEncoding(m))
            }
            objc_registerClassPair(made)
            sub = made
        }
        guard let sub else { return }
        object_setClass(content, sub)
        objc_setAssociatedObject(content, barKey, bar, .OBJC_ASSOCIATION_RETAIN_NONATOMIC)
    }
}

/// PhotoPicking is the photo the keyboard bar asked for: from the library or the camera, saved
/// beside the note with nt paste as a JPEG (nt takes no HEIC), on a line of its own at the
/// cursor.
struct PhotoPicking: ViewModifier {
    @Environment(Workspace.self) private var ws
    @State private var library = false
    @State private var camera = false
    @State private var item: PhotosPickerItem?

    func body(content: Content) -> some View {
        @Bindable var ws = ws
        content
            .confirmationDialog("Add a photo", isPresented: $ws.picking) {
                Button("Photo Library") { library = true }
                if UIImagePickerController.isSourceTypeAvailable(.camera) {
                    Button("Take Photo") { camera = true }
                }
            }
            .photosPicker(isPresented: $library, selection: $item, matching: .images)
            .onChange(of: item) {
                guard let item else { return }
                self.item = nil
                Task {
                    guard let data = try? await item.loadTransferable(type: Data.self) else {
                        ws.say("That photo could not be read")
                        return
                    }
                    await add(data)
                }
            }
            .fullScreenCover(isPresented: $camera) {
                CameraPicker { data in Task { await add(data) } }
                    .ignoresSafeArea()
            }
    }

    private func add(_ data: Data) async {
        guard let editor = ws.editor, let jpeg = UIImage(data: data)?.jpegData(compressionQuality: 0.85) else {
            ws.say("That photo could not be read")
            return
        }
        await editor.paste(name: "photo.jpg", mime: "image/jpeg", data: jpeg, at: await editor.cursor())
    }
}

/// CameraPicker is the camera, for a photo of the moment.
struct CameraPicker: UIViewControllerRepresentable {
    let took: (Data) -> Void
    @Environment(\.dismiss) private var dismiss

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let p = UIImagePickerController()
        p.sourceType = .camera
        p.delegate = context.coordinator
        return p
    }

    func updateUIViewController(_ p: UIImagePickerController, context: Context) {}

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    final class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
        let parent: CameraPicker

        init(_ parent: CameraPicker) { self.parent = parent }

        func imagePickerController(_ picker: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
            if let image = info[.originalImage] as? UIImage, let data = image.jpegData(compressionQuality: 0.85) {
                parent.took(data)
            }
            parent.dismiss()
        }

        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
            parent.dismiss()
        }
    }
}
