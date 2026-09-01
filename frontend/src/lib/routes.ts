export function routeClass(route: string): string {
  if (route === 'Transcode') return 'b-transcode';
  if (route === 'Reencode') return 'b-reencode';
  if (route === 'Encode') return 'b-encode';
  return 'b-skip';
}

export function routeTitle(route: string): string {
  if (route === 'Transcode') return 'JPEG transcode';
  if (route === 'Reencode') return 'JXL reencode';
  if (route === 'Encode') return 'Pixel encode';
  return 'Skipped';
}
