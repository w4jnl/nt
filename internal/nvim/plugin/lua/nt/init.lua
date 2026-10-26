-- nt in nvim (docs/design.md §4, nvim). In a markdown note inside the notes repository:
--   <leader>ip   pastes the clipboard's image: nt paste saves it beside the note, the line
--                that shows it goes below the cursor.
--   While a note has unsaved changes, sync commits but does not merge (nt holds .git/nt-edit),
--   so a merge never lands under text you have not written yet.
--   A note sync changed on disk is reloaded when it has no unsaved changes.
--   Leaving nvim after writing a note starts a sync, so the change goes out at once.
-- Installed by `nt install --nvim`; every action is the nt command line, nothing else.
local M = {}

local defaults = {
  cmd = "nt", -- the nt binary
  keys = { paste = "<leader>ip" }, -- false turns a key off
  reload = 3000, -- ms between checks for notes changed on disk; 0 turns them off
}

M.opts = vim.deepcopy(defaults)

local root -- the repository's folder, real path; false when nt finds none
local hold -- the job holding .git/nt-edit
local written = false -- a note was written in this session
local timer

local function repo()
  if root == nil then
    local ok, res = pcall(function()
      return vim.system({ M.opts.cmd, "_editor", "repo" }, { text = true }):wait()
    end)
    root = ok and res.code == 0 and vim.uv.fs_realpath(vim.trim(res.stdout)) or false
  end
  return root
end

-- is_note reports whether buf is a markdown file in the repository; the file need not exist yet.
local function is_note(buf)
  local name = vim.api.nvim_buf_get_name(buf)
  if name == "" or not name:match("%.md$") or vim.bo[buf].buftype ~= "" then
    return false
  end
  local r = repo()
  local dir = r and vim.uv.fs_realpath(vim.fs.dirname(name))
  return dir ~= nil and dir ~= false and (dir == r or vim.startswith(dir, r .. "/"))
end

local function notes()
  local out = {}
  for _, b in ipairs(vim.api.nvim_list_bufs()) do
    if vim.api.nvim_buf_is_loaded(b) and vim.b[b].nt_note then
      out[#out + 1] = b
    end
  end
  return out
end

-- update_hold holds .git/nt-edit while a note has unsaved changes and lets it go when none has.
local function update_hold()
  local dirty = false
  for _, b in ipairs(notes()) do
    dirty = dirty or vim.bo[b].modified
  end
  if dirty and not hold then
    local id
    id = vim.fn.jobstart({ M.opts.cmd, "_editor", "hold" }, {
      on_exit = function()
        if hold == id then
          hold = nil
        end
      end,
    })
    hold = id > 0 and id or nil
  elseif not dirty and hold then
    vim.fn.chanclose(hold, "stdin") -- nt lets the hold go when its input ends
    hold = nil
  end
end

-- reload reads again the notes that changed on disk and have no unsaved changes.
local function reload()
  if vim.api.nvim_get_mode().mode ~= "n" then
    return
  end
  for _, b in ipairs(notes()) do
    if not vim.bo[b].modified then
      vim.cmd("checktime " .. b)
    end
  end
end

local function paste(buf)
  vim.ui.input({ prompt = "image name (empty: image): " }, function(name)
    if name == nil then
      return -- escape
    end
    local cmd = { M.opts.cmd, "paste", vim.api.nvim_buf_get_name(buf) }
    if vim.trim(name) ~= "" then
      vim.list_extend(cmd, { "--name", name })
    end
    local res = vim.system(cmd, { text = true }):wait()
    if res.code ~= 0 then
      vim.notify(vim.trim(res.stderr), vim.log.levels.ERROR)
      return
    end
    vim.api.nvim_put({ vim.trim(res.stdout) }, "l", true, true)
  end)
end

local function attach(buf)
  if vim.b[buf].nt_note ~= nil then
    return
  end
  vim.b[buf].nt_note = is_note(buf)
  if not vim.b[buf].nt_note then
    return
  end
  vim.bo[buf].autoread = true
  if M.opts.keys.paste then
    vim.keymap.set("n", M.opts.keys.paste, function()
      paste(buf)
    end, { buffer = buf, desc = "nt: paste the clipboard's image" })
  end
  if not timer and M.opts.reload > 0 then
    timer = vim.uv.new_timer()
    timer:start(M.opts.reload, M.opts.reload, vim.schedule_wrap(reload))
  end
end

function M.setup(opts)
  M.opts = vim.tbl_deep_extend("force", vim.deepcopy(defaults), opts or {})
  if timer then
    timer:stop()
    timer:close()
    timer = nil
  end
  local g = vim.api.nvim_create_augroup("nt", { clear = true })
  vim.api.nvim_create_autocmd({ "BufReadPost", "BufNewFile" }, {
    group = g,
    pattern = "*.md",
    callback = function(ev)
      attach(ev.buf)
    end,
  })
  vim.api.nvim_create_autocmd("BufModifiedSet", {
    group = g,
    callback = function(ev)
      if vim.b[ev.buf].nt_note then
        update_hold()
      end
    end,
  })
  vim.api.nvim_create_autocmd("BufWritePost", {
    group = g,
    pattern = "*.md",
    callback = function(ev)
      if vim.b[ev.buf].nt_note then
        written = true
        update_hold()
      end
    end,
  })
  vim.api.nvim_create_autocmd({ "BufDelete", "BufWipeout" }, {
    group = g,
    callback = function()
      vim.schedule(update_hold)
    end,
  })
  vim.api.nvim_create_autocmd("VimLeavePre", {
    group = g,
    callback = function()
      if hold then
        local id = hold
        vim.fn.chanclose(id, "stdin")
        vim.fn.jobwait({ id }, 1000) -- the hold is gone before sync looks for it
      end
      if written then
        vim.fn.jobstart({ M.opts.cmd, "_editor", "kick" }, { detach = true })
      end
    end,
  })
  for _, b in ipairs(vim.api.nvim_list_bufs()) do
    if vim.api.nvim_buf_is_loaded(b) and vim.api.nvim_buf_get_name(b):match("%.md$") then
      vim.b[b].nt_note = nil
      attach(b)
    end
  end
end

return M
