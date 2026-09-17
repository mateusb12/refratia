import { useEffect, useState } from 'react'
import {
  Database,
  FileSearch,
  Plus,
  Trash2,
} from 'lucide-react'

interface CaseOption {
  caseId: string
  patientName: string
}

interface SavedExam {
  caseId: string
  patientName: string
  filename: string
  exam?: string
  eye?: string
  contentType?: string
  path: string
  signedUrl?: string
  size: number
  lastModified?: string
  referenced: boolean
}

interface SavedExamsPageProps {
  apiUrl: string
  mutationToken: string
  cases: CaseOption[]
  examLabels: Record<string, string>
}

function fileIcon(fileName: string) {
  const extension = fileName.split('.').pop()?.toLowerCase()

  return `${import.meta.env.BASE_URL}${
    extension === 'pdf'
      ? 'pdf.png'
      : extension === 'png'
        ? 'png.png'
        : 'jpeg.png'
  }`
}

function directUploadContentType(file: File) {
  if (file.type) return file.type

  const extension = file.name.split('.').pop()?.toLowerCase()

  const mapping: Record<string, string> = {
    pdf: 'application/pdf',
    doc: 'application/msword',
    docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    xls: 'application/vnd.ms-excel',
    xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    jpg: 'image/jpeg',
    jpeg: 'image/jpeg',
    png: 'image/png',
  }

  return mapping[extension ?? ''] ?? 'application/octet-stream'
}

export default function SavedExamsPage({
  apiUrl,
  mutationToken,
  cases,
  examLabels,
}: SavedExamsPageProps) {
  const [exams, setExams] = useState<SavedExam[]>([])
  const [loading, setLoading] = useState(false)
  const [storageError, setStorageError] = useState('')
  const [search, setSearch] = useState('')

  const [uploadCaseId, setUploadCaseId] = useState('')
  const [uploadBusy, setUploadBusy] = useState(false)
  const [uploadMessage, setUploadMessage] = useState('')

  const [deletePath, setDeletePath] = useState<string | null>(null)

  async function loadExams() {
    setLoading(true)
    setStorageError('')

    try {
      const response = await fetch(`${apiUrl}/api/exams`)
      const result = await response.json()

      if (!response.ok || !Array.isArray(result.exams)) {
        throw new Error(
          result.error ?? 'Não foi possível listar os exames.',
        )
      }

      setExams(result.exams as SavedExam[])
    } catch (examListError) {
      setStorageError(
        examListError instanceof Error
          ? examListError.message
          : 'Não foi possível listar os exames.',
      )
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadExams()
  }, [])

  async function uploadExam(file: File) {
    if (!uploadCaseId) return

    const contentType = directUploadContentType(file)

    setUploadBusy(true)
    setUploadMessage('Preparando upload…')

    try {
      const response = await fetch(
        `${apiUrl}/api/exams/upload-url`,
        {
          method: 'POST',
          headers: {
            Authorization: `Bearer ${mutationToken}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            caseId: uploadCaseId,
            filename: file.name,
            contentType,
            size: file.size,
          }),
        },
      )

      const result = await response.json()

      if (!response.ok || !result.uploadUrl) {
        throw new Error(
          result.error ?? 'Não foi possível preparar o upload.',
        )
      }

      setUploadMessage('Enviando diretamente para o Tigris…')

      const upload = await fetch(result.uploadUrl, {
        method: 'PUT',
        headers: {
          'Content-Type': contentType,
        },
        body: file,
      })

      if (!upload.ok) {
        throw new Error(
          `Tigris recusou o upload (${upload.status}).`,
        )
      }

      setUploadMessage('Arquivo salvo no Tigris.')

      await loadExams()
    } catch (examUploadError) {
      setUploadMessage(
        examUploadError instanceof Error
          ? examUploadError.message
          : 'Não foi possível enviar o arquivo.',
      )
    } finally {
      setUploadBusy(false)
    }
  }

  async function deleteExam(item: SavedExam) {
    const confirmed = window.confirm(
      `Excluir permanentemente ${item.filename} do Tigris?`,
    )

    if (!confirmed) return

    setDeletePath(item.path)
    setStorageError('')

    try {
      const response = await fetch(`${apiUrl}/api/exams`, {
        method: 'DELETE',
        headers: {
          Authorization: `Bearer ${mutationToken}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          caseId: item.caseId,
          path: item.path,
        }),
      })

      const responseBody = await response.text()

      let responseData: { error?: string } = {}

      if (responseBody.trim()) {
        try {
          responseData = JSON.parse(responseBody)
        } catch {
          throw new Error(
            `DELETE /api/exams retornou HTTP ${response.status} com resposta não-JSON: ${responseBody.slice(0, 300)}`,
          )
        }
      }

      if (!response.ok) {
        throw new Error(
          responseData.error ??
            `DELETE /api/exams falhou com HTTP ${response.status}${response.statusText ? ` ${response.statusText}` : ''}`,
        )
      }

      setExams((current) =>
        current.filter((exam) => exam.path !== item.path),
      )
    } catch (examDeleteError) {
      setStorageError(
        examDeleteError instanceof Error
          ? examDeleteError.message
          : 'Não foi possível excluir o arquivo.',
      )
    } finally {
      setDeletePath(null)
    }
  }

  const normalizedSearch =
    search.trim().toLocaleLowerCase('pt-BR')

  const filteredExams = exams.filter((item) => {
    if (!normalizedSearch) return true

    const translatedExam =
      item.exam ? examLabels[item.exam] : undefined

    const searchable = [
      item.patientName,
      item.filename,
      item.exam,
      item.eye,
      translatedExam,
    ]
      .filter(Boolean)
      .join(' ')
      .toLocaleLowerCase('pt-BR')

    return searchable.includes(normalizedSearch)
  })

  return (
    <section className="mt-5 rounded-2xl border border-border bg-surface p-6 shadow-sm max-[580px]:p-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <span className="text-primary text-xs font-bold tracking-[0.13em]">
            STORAGE · TIGRIS
          </span>

          <h2 className="mb-0 mt-1 font-display text-xl">
            Exames salvos
          </h2>

          <p className="mb-0 mt-2 text-sm text-text-secondary">
            Adicione, abra ou exclua arquivos diretamente do storage.
          </p>
        </div>

        <span className="rounded-full border border-border bg-surface-muted px-3 py-1 text-xs font-bold text-text-secondary">
          {loading
            ? 'Carregando…'
            : `${exams.length} arquivo(s)`}
        </span>
      </div>

      <div className="mt-5 flex flex-wrap items-end gap-3 rounded-xl border border-primary-border bg-primary-soft/30 p-4">
        <label className="min-w-[260px] flex-1">
          <span className="mb-1.5 block text-xs font-bold text-text-muted">
            PACIENTE
          </span>

          <select
            className="w-full rounded-xl border border-border bg-surface px-3 py-2.5 text-sm text-text-primary"
            onChange={(event) => {
              setUploadCaseId(event.target.value)
              setUploadMessage('')
            }}
            value={uploadCaseId}
          >
            <option value="">
              Selecione um paciente
            </option>

            {cases.map((item) => (
              <option
                key={item.caseId}
                value={item.caseId}
              >
                {item.patientName}
              </option>
            ))}
          </select>
        </label>

        <label
          className={[
            'inline-flex items-center gap-2 rounded-xl border px-4 py-2.5 text-sm font-bold',
            uploadCaseId && !uploadBusy
              ? 'cursor-pointer border-primary-border bg-primary-soft text-primary hover:border-primary'
              : 'cursor-not-allowed border-border bg-surface-muted text-text-muted',
          ].join(' ')}
        >
          <Plus size={17} />

          {uploadBusy
            ? 'Enviando…'
            : 'Adicionar exame'}

          <input
            accept=".pdf,.doc,.docx,.xls,.xlsx,.jpg,.jpeg,.png"
            className="sr-only"
            disabled={!uploadCaseId || uploadBusy}
            onChange={(event) => {
              const file = event.target.files?.[0]

              if (file) {
                void uploadExam(file)
              }

              event.target.value = ''
            }}
            type="file"
          />
        </label>
      </div>

      {uploadMessage && (
        <p className="mb-0 mt-3 rounded-lg border border-border bg-surface-muted p-3 text-xs font-semibold text-text-secondary">
          {uploadMessage}
        </p>
      )}

      <div className="mt-5 flex items-center gap-3 rounded-xl border border-border bg-surface-muted px-4 py-3">
        <FileSearch
          className="text-text-muted"
          size={18}
        />

        <input
          className="min-w-0 flex-1 border-0 bg-transparent text-sm text-text-primary outline-none"
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Buscar paciente, exame ou arquivo…"
          type="search"
          value={search}
        />
      </div>

      {storageError && (
        <div className="mt-4 rounded-xl border border-danger/30 bg-danger-soft p-4 text-sm font-semibold text-danger">
          {storageError}
        </div>
      )}

      {loading ? (
        <p className="mt-5 text-sm text-text-secondary">
          Consultando Tigris…
        </p>
      ) : filteredExams.length === 0 ? (
        <div className="mt-5 rounded-xl border border-border bg-surface-muted p-8 text-center">
          <Database
            className="mx-auto text-text-muted"
            size={30}
          />

          <strong className="mt-3 block text-sm">
            Nenhum arquivo encontrado
          </strong>
        </div>
      ) : (
        <div className="mt-5 overflow-x-auto rounded-xl border border-border">
          <table className="w-full min-w-[950px] border-collapse text-left">
            <thead className="bg-surface-muted text-xs text-text-secondary">
              <tr>
                <th className="px-4 py-3">Paciente</th>
                <th className="px-4 py-3">Exame</th>
                <th className="px-4 py-3">Olho</th>
                <th className="px-4 py-3">Arquivo</th>
                <th className="px-4 py-3 text-right">Ações</th>
              </tr>
            </thead>

            <tbody>
              {filteredExams.map((item) => {
                const examLabel =
                  item.exam
                    ? examLabels[item.exam] ??
                      item.exam.replaceAll('_', ' ')
                    : 'Não processado'

                const eye =
                  item.eye === 'OS'
                    ? 'OE'
                    : item.eye || '—'

                const deleting =
                  deletePath === item.path

                return (
                  <tr
                    className="border-t border-border hover:bg-surface-muted/60"
                    key={`${item.caseId}-${item.path}`}
                  >
                    <td className="px-4 py-3">
                      <strong className="block text-sm">
                        {item.patientName}
                      </strong>

                      <span className="text-[10px] text-text-muted">
                        {item.caseId}
                      </span>
                    </td>

                    <td className="px-4 py-3 text-sm">
                      <span
                        className={
                          item.referenced
                            ? 'text-text-secondary'
                            : 'font-semibold text-warning'
                        }
                      >
                        {examLabel}
                      </span>
                    </td>

                    <td className="px-4 py-3 text-sm">
                      {eye}
                    </td>

                    <td className="max-w-[350px] px-4 py-3">
                      <div className="flex min-w-0 items-center gap-3">
                        <img
                          alt=""
                          className="h-8 w-8 flex-none object-contain"
                          src={fileIcon(item.filename)}
                        />

                        <div className="min-w-0">
                          <span
                            className="block truncate font-mono text-xs"
                            title={item.filename}
                          >
                            {item.filename}
                          </span>

                          {item.size > 0 && (
                            <span className="text-[10px] text-text-muted">
                              {(item.size / 1024 / 1024).toFixed(2)} MB
                            </span>
                          )}
                        </div>
                      </div>
                    </td>

                    <td className="px-4 py-3">
                      <div className="flex justify-end gap-2">
                        {item.signedUrl && (
                          <a
                            className="rounded-lg border border-primary-border bg-primary-soft px-3 py-2 text-xs font-bold text-primary"
                            href={item.signedUrl}
                            rel="noreferrer"
                            target="_blank"
                          >
                            Abrir
                          </a>
                        )}

                        <button
                          className="inline-flex items-center gap-1.5 rounded-lg border border-danger/30 bg-danger-soft px-3 py-2 text-xs font-bold text-danger hover:border-danger"
                          disabled={deleting}
                          onClick={() =>
                            void deleteExam(item)
                          }
                          type="button"
                        >
                          <Trash2 size={14} />

                          {deleting
                            ? 'Excluindo…'
                            : 'Excluir'}
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
