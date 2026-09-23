// Copies text to the clipboard. When the text is still being loaded, ClipboardItem
// takes the pending value so the copy stays tied to the click that started it.
export async function copyText(text: string | Promise<string>): Promise<void> {
  const clipboard = navigator.clipboard;
  if (typeof text !== 'string' && typeof ClipboardItem !== 'undefined' && typeof clipboard.write === 'function') {
    await clipboard.write([new ClipboardItem({ 'text/plain': text.then(value => new Blob([value], { type: 'text/plain' })) })]);
    return;
  }
  await clipboard.writeText(await text);
}

export function downloadText(filename: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }));
  const link = document.createElement('a'); link.href = url; link.download = filename;
  document.body.appendChild(link); link.click(); link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
