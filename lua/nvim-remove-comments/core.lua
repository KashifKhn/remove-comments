local ts = vim.treesitter
local config = require("nvim-remove-comments.config")

local M = {}

function M.remove_comments()
	local bufnr = vim.api.nvim_get_current_buf()
	local lang = vim.bo[bufnr].filetype

	if lang == "" then
		return
	end

	local ok, parser = pcall(ts.get_parser, bufnr, lang)
	if not ok or not parser then
		return
	end

	local root = parser:parse()[1]:root()
	local query_str = config.queries[lang]
	local query = ts.query.parse(lang, query_str or [[ (comment) @comment ]])

	local lines_to_delete = {}

	for _, node in query:iter_captures(root, bufnr, 0, -1) do
		local srow, scol, erow, ecol = node:range()

		if srow == 0 and vim.startswith(vim.fn.getline(1), "#!") then
			goto continue
		end

		local lines = vim.api.nvim_buf_get_lines(bufnr, srow, erow + 1, false)

		if srow == erow then
			local line = lines[1]

			if scol == 0 and ecol == #line then
				lines_to_delete[srow] = true
			else
				local before = line:sub(1, scol)
				local after = line:sub(ecol + 1)
				vim.api.nvim_buf_set_lines(bufnr, srow, srow + 1, false, { before .. after })
			end
		else
			for i = srow, erow do
				lines_to_delete[i] = true
			end
		end

		::continue::
	end

	local rows = {}
	for row in pairs(lines_to_delete) do
		table.insert(rows, row)
	end
	table.sort(rows, function(a, b)
		return a > b
	end)

	for _, row in ipairs(rows) do
		vim.api.nvim_buf_set_lines(bufnr, row, row + 1, false, {})
	end

	vim.lsp.buf.format({ async = true })
end

return M
