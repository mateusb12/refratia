export function fileIcon(fileName: string) {
  const extension = fileName.split('.').pop()?.toLowerCase()
  return `${import.meta.env.BASE_URL}${extension === 'pdf' ? 'pdf.png' : extension === 'png' ? 'png.png' : 'jpeg.png'}`
}
