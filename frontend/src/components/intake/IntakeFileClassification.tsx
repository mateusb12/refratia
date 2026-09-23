import { useEffect, useState } from 'react'
import { FileSearch, LoaderCircle, TriangleAlert, X } from 'lucide-react'

export type IntakeExamType =
  | 'pentacam_corneal_tomography'
  | 'iol_calculation'
  | 'specular_microscopy'
  | 'fundus_retinography'
  | 'oct_retina'

export interface IntakeClassification {
  sha256: string
  examType: IntakeExamType | ''
  eye: 'OD' | 'OS' | 'AO' | ''
}

interface Inspection {
  filename: string
  sha256: string
  contentType: string
  size: number
  pages?: number
  width?: number
  height?: number
  recognizedExamType?: IntakeExamType
  recognizedEye?: 'OD' | 'OS' | 'AO'
  candidates: Array<{ examType: IntakeExamType; score: number; reason: string }>
}

const labels: Record<IntakeExamType, string> = {
  pentacam_corneal_tomography: 'Pentacam',
  iol_calculation: 'Biometria / EyeSuite',
  specular_microscopy: 'Microscopia especular',
  fundus_retinography: 'Retinografia',
  oct_retina: 'OCT de retina',
}

const types = Object.keys(labels) as IntakeExamType[]

function metadataLine(item: Inspection) {
  const details = [`${(item.size / 1024).toFixed(0)} KB`]
  if (item.pages) details.push(`${item.pages} páginas`)
  if (item.width && item.height) details.push(`${item.width} × ${item.height}px`)
  return details.join(' · ')
}

function fileIcon(fileName: string) {
  const extension = fileName.split('.').pop()?.toLowerCase()
  return `${import.meta.env.BASE_URL}${extension === 'pdf' ? 'pdf.png' : extension === 'png' ? 'png.png' : 'jpeg.png'}`
}

export default function IntakeFileClassification({
  apiUrl,
  files,
  localPreviews,
  value,
  onChange,
  onRemove,
}: {
  apiUrl: string
  files: File[]
  localPreviews: Record<string, string>
  value: Record<string, IntakeClassification>
  onChange: (next: Record<string, IntakeClassification>) => void
  onRemove: (file: File) => void
}) {
  const [items, setItems] = useState<Inspection[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [draggedSha, setDraggedSha] = useState<string | null>(null)

  useEffect(() => {
    if (!files.length || !apiUrl) return
    const controller = new AbortController()
    const body = new FormData()
    files.forEach((file) => body.append('files', file))
    setLoading(true)
    setError('')
    fetch(`${apiUrl}/api/intakes/inspect`, { method: 'POST', body, signal: controller.signal })
      .then(async (response) => {
        const payload = await response.json()
        if (!response.ok) {
          throw new Error(
            response.status === 404
              ? 'O backend publicado ainda não possui a rota de inspeção (/api/intakes/inspect). Publique o backend atualizado.'
              : payload.error ?? `Não foi possível inspecionar os arquivos (HTTP ${response.status}).`,
          )
        }
        return payload as { files: Inspection[] }
      })
      .then((payload) => {
        const inspectedItems = payload.files
        setItems(inspectedItems)
        const next: Record<string, IntakeClassification> = {}
        inspectedItems.forEach((item) => {
          const previous = value[item.sha256]
          if (previous) {
            next[item.sha256] = previous
            return
          }
          const suggestion = item.candidates.find((candidate) => candidate.score > 0)
          next[item.sha256] = {
            sha256: item.sha256,
            examType: item.recognizedExamType || suggestion?.examType || '',
            eye: item.recognizedEye || '',
          }
        })
        onChange(next)
      })
      .catch((inspectionError) => {
        if (!controller.signal.aborted) setError(inspectionError instanceof Error ? inspectionError.message : 'Falha na inspeção.')
      })
      .finally(() => setLoading(false))
    return () => controller.abort()
    // A nova seleção de arquivos é a única mudança que deve refazer a inspeção.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [files, apiUrl])

  if (!files.length) return null

  const eyeGroups = [
    { key: 'OD', label: 'Olho direito', code: 'OD', detail: 'Arraste os exames para este olho' },
    { key: 'OS', label: 'Olho esquerdo', code: 'OS', detail: 'Arraste os exames para este olho' },
    { key: 'AO', label: 'Ambos os olhos', code: 'AO', detail: 'Exames que se aplicam aos dois olhos' },
  ] as const

  const renderFile = (item: Inspection) => {
    const classification = value[item.sha256] ?? { sha256: item.sha256, examType: '', eye: '' }
    const suggestion = item.candidates.find((candidate) => candidate.score > 0)
    const selectedFile = files.find((file) => file.name === item.filename)
    const recognized = Boolean(item.recognizedExamType && item.recognizedEye)
    return (
      <div
        className="relative grid w-fit max-w-full cursor-grab grid-cols-[auto_minmax(0,1fr)] gap-3 border border-transparent px-3 py-3 transition-[background,border-color,box-shadow,transform] duration-150 hover:-translate-y-px hover:border-primary hover:bg-primary-soft/20 hover:shadow-sm active:cursor-grabbing sm:grid-cols-[auto_minmax(0,auto)_220px] sm:items-center"
        draggable
        key={item.sha256}
        onDragEnd={() => setDraggedSha(null)}
        onDragStart={() => setDraggedSha(item.sha256)}
      >
        <div className="grid h-12 w-12 place-items-center overflow-hidden rounded-lg border border-border bg-surface">
          {item.contentType.startsWith('image/') && localPreviews[item.filename]
            ? <img alt="" className="h-full w-full object-cover" src={localPreviews[item.filename]} />
            : <img alt="" className="h-10 w-10 object-contain" src={fileIcon(item.filename)} />}
        </div>
        <div className="min-w-0">
          <strong className="block truncate text-sm" title={item.filename}>{item.filename}</strong>
          <span className="block truncate text-xs text-text-muted" title={`${metadataLine(item)}${suggestion ? ` · sugestão: ${labels[suggestion.examType]}` : ''}`}>
            {metadataLine(item)}{suggestion ? ` · sugestão: ${labels[suggestion.examType]}` : ''}
          </span>
        </div>
        <div className="col-span-2 grid min-w-0 grid-cols-1 gap-2 sm:contents">
          <select
            aria-label={`Tipo de ${item.filename}`}
            className="min-w-0 rounded-lg border border-border bg-surface px-3 py-2 text-sm"
            onChange={(event) => onChange({ ...value, [item.sha256]: { ...classification, examType: event.target.value as IntakeExamType | '' } })}
            value={classification.examType}
          >
            <option value="">Escolha o tipo…</option>
            {types.map((type) => <option key={type} value={type}>{labels[type]}</option>)}
          </select>
          {recognized && (
            <span className="col-span-2 text-[10px] text-success sm:col-span-3">
              Reconhecido pela nomenclatura padronizada; você pode corrigir.
            </span>
          )}
        </div>
        <button
          aria-label={`Remover ${item.filename}`}
          className="absolute right-2 top-2 grid h-8 w-8 place-items-center rounded-full text-text-muted hover:bg-danger-soft hover:text-danger"
          disabled={!selectedFile}
          onClick={() => { if (selectedFile) onRemove(selectedFile) }}
          type="button"
        >
          <X size={15} />
        </button>
      </div>
    )
  }

  return (
    <div className="mt-4 rounded-xl border border-border bg-surface-muted/50">
      <div className="flex items-start gap-3 border-b border-border px-4 py-3.5">
        <FileSearch className="mt-0.5 flex-none text-primary" size={17} />
        <div>
          <strong className="text-sm">Confirme o tipo de cada arquivo</strong>
          <p className="mb-0 mt-1 text-xs leading-relaxed text-text-secondary">A sugestão usa metadados técnicos e, quando o arquivo já segue a nomenclatura padronizada, usa o nome como sinal. Você pode corrigir qualquer classificação.</p>
        </div>
        {loading && <LoaderCircle className="ml-auto animate-spin text-primary" size={16} />}
      </div>
      {error && <div className="flex gap-2 border-b border-warning/30 bg-warning-soft px-4 py-3 text-xs text-warning"><TriangleAlert size={15} />{error}</div>}
      <div className="grid gap-3 p-3 lg:grid-cols-2">
        {eyeGroups.map((group) => {
          const groupItems = items.filter((item) => {
            const eye = value[item.sha256]?.eye ?? ''
            return eye === group.code || (eye === '' && group.code === 'OD')
          })
          return (
            <section
              className="min-h-[220px] overflow-hidden rounded-xl border border-border bg-surface/40 transition-colors"
              key={group.key}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault()
                if (draggedSha) {
                  onChange({ ...value, [draggedSha]: { ...value[draggedSha], eye: group.code } })
                  setDraggedSha(null)
                }
              }}
            >
              <div className="flex items-center justify-between gap-3 border-b border-border bg-surface-muted/60 px-3 py-2.5">
                <div>
                  <strong className="block text-sm">{group.label}{group.code && ` (${group.code})`}</strong>
                  <span className="text-[11px] text-text-muted">{group.detail}</span>
                </div>
                <span className="rounded-full border border-border bg-surface px-2 py-1 text-[10px] font-bold text-text-secondary">{groupItems.length}</span>
              </div>
              <div className="min-h-[165px] divide-y divide-border">
                {groupItems.length ? groupItems.map((item) => renderFile(item)) : (
                  <div className="grid min-h-[165px] place-items-center p-6 text-center text-xs text-text-muted">
                    Arraste um exame para cá
                  </div>
                )}
              </div>
            </section>
          )
        })}
      </div>
      {items.length > 0 && items.every((item) => item.recognizedExamType && item.recognizedEye) && (
        <p className="mb-0 border-t border-border px-4 py-3 text-xs text-success">
          Todos os arquivos foram reconhecidos pela nomenclatura padronizada.
        </p>
      )}
    </div>
  )
}
