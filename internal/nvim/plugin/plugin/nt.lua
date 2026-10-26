-- Sets nt up with its defaults when nvim loads the plugin; require("nt").setup(opts) (lazy.nvim's
-- opts) sets it up again with yours.
if vim.g.loaded_nt then
  return
end
vim.g.loaded_nt = true
require("nt").setup()
