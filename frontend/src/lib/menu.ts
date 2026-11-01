// Result rows (Running view, History) opt into the native "result-row" menu
// registered in main.go. The menu data names the row — "r<seq>" for a run
// result, "h<index>" for a history entry — and App.svelte resolves it to a file.
export const resultMenuStyle = (token: string): string =>
  `--custom-contextmenu: result-row; --custom-contextmenu-data: ${token}; --default-contextmenu: hide`;
