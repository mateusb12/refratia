import { useState } from 'react'
import {
  Check,
  ChevronDown,
  FileSearch,
  LoaderCircle,
  Trash2,
  UploadCloud,
  X,
} from 'lucide-react'
import { examContracts } from '../../contracts/exam-contracts'

const API_URL = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '')

interface BenchmarkField {
  key: string
  label: string
  unit?: string
  found: boolean
  value?: string
}

interface BenchmarkEvent {
  type: 'progress' | 'task_progress' | 'result' | 'error'
  percent?: number
  message?: string
  examType?: string
  eye?: string
  elapsedMs?: number
  fields?: BenchmarkField[]
  extracted?: number
  total?: number
  supported?: boolean
  error?: string
  taskId?: string
  taskLabel?: string
  taskStatus?: 'running' | 'completed' | 'failed'
  taskPercent?: number
}

interface BenchmarkTask {
  taskId: string
  taskLabel: string
  taskStatus: 'running' | 'completed' | 'failed'
  taskPercent: number
}

const benchmarkSteps = [
  'Preparar documento',
  'Processar páginas',
  'Extrair dados',
  'Atividades específicas',
]

function duration(ms?: number) {
  if (ms == null) return ''

  if (ms < 1000) {
    return `${ms} ms`
  }

  const seconds = ms / 1000

  if (seconds < 60) {
    return `${seconds.toFixed(1)} s`
  }

  const minutes = Math.floor(seconds / 60)
  const rest = Math.round(seconds % 60)

  return `${minutes}m ${rest}s`
}

function filenameMetadata(name: string) {
  const stem = name.replace(/\.[^.]+$/, '')
  const parts = stem.split('__')

  if (parts.length >= 3) {
    return {
      type: parts[0].toUpperCase(),
      eye: parts[1].toUpperCase(),
    }
  }

  const spaced = stem.toUpperCase().match(
    /^(AUTO|BIOMETRIA|PENTACAM|RETINA|OCT|EYESUITE|MICROSCOPIA(?:\s+ESPECULAR)?)[\s_-]+(OD|OE|OS|AO)(?:\s|$)/,
  )

  if (!spaced) return null

  const typeMap: Record<string, string> = {
    AUTO: 'AUTO',
    BIOMETRIA: 'EYESUITE',
    PENTACAM: 'PENTACAM',
    RETINA: 'RETINA',
    OCT: 'OCT',
    EYESUITE: 'EYESUITE',
    MICROSCOPIA: 'MICROSCOPIA_ESPECULAR',
    'MICROSCOPIA ESPECULAR': 'MICROSCOPIA_ESPECULAR',
  }

  return {
    type: typeMap[spaced[1]] ?? spaced[1],
    eye: spaced[2] === 'OE' ? 'OS' : spaced[2],
  }
}

export default function BenchmarkOcrPage() {
  const [file, setFile] = useState<File | null>(null)
  const [dragging, setDragging] = useState(false)
  const [busy, setBusy] = useState(false)

  const [progress, setProgress] = useState(0)
  const [progressMessage, setProgressMessage] = useState('')
  const [tasks, setTasks] = useState<Record<string, BenchmarkTask>>({})

  const [examType, setExamType] = useState('')
  const [eye, setEye] = useState('')

  const [fields, setFields] = useState<BenchmarkField[]>([])
  const [extracted, setExtracted] = useState(0)
  const [total, setTotal] = useState(0)
  const [elapsedMs, setElapsedMs] = useState<number | undefined>()
  const [supported, setSupported] = useState<boolean | undefined>()

  const [error, setError] = useState('')
  const [showExamDetails, setShowExamDetails] = useState(false)

  function resetRun() {
    setProgress(0)
    setProgressMessage('')
    setTasks({})
    setFields([])
    setExtracted(0)
    setTotal(0)
    setElapsedMs(undefined)
    setSupported(undefined)
    setError('')
  }

  function selectBenchmarkFile(selected: File | null) {
    if (!selected) return

    const meta = filenameMetadata(selected.name)

    if (!meta) {
      setError(
        'Nome fora do padrão TIPO__LATERALIDADE__PACIENTE__DATAHORA.ext',
      )
      return
    }

    setFile(selected)
    setExamType(meta.type)
    setEye(meta.eye)
    resetRun()
  }

  function clearBenchmarkFile() {
    setFile(null)
    setExamType('')
    setEye('')
    resetRun()
  }

  async function runBenchmark() {
    if (!file || busy) return

    resetRun()

    if (!API_URL) {
      setError('VITE_API_URL não está configurada.')
      return
    }

    const meta = filenameMetadata(file.name)

    if (!meta) {
      setError(
        'Nome fora do padrão TIPO__LATERALIDADE__PACIENTE__DATAHORA.ext',
      )
      return
    }

    setExamType(meta.type)
    setEye(meta.eye)

    setBusy(true)
    setProgress(1)
    setProgressMessage('Enviando arquivo')

    try {
      const data = new FormData()
      data.append('files', file)

      const response = await fetch(
        `${API_URL}/api/benchmark/extract-fields`,
        {
          method: 'POST',
          headers: {
            Accept: 'application/x-ndjson',
          },
          body: data,
        },
      )

      const contentType =
        response.headers.get('content-type') ?? ''

      if (!contentType.includes('application/x-ndjson')) {
        const text = await response.text()

        let message = `Falha no benchmark (HTTP ${response.status})`

        try {
          const parsed = JSON.parse(text)

          if (typeof parsed?.error === 'string') {
            message = parsed.error
          }
        } catch {
          // mantém fallback
        }

        throw new Error(message)
      }

      if (!response.body) {
        throw new Error(
          'O backend não disponibilizou o stream.',
        )
      }

      const reader = response.body.getReader()
      const decoder = new TextDecoder()

      let buffer = ''

      const consume = (line: string) => {
        const trimmed = line.trim()
        if (!trimmed) return

        const event =
          JSON.parse(trimmed) as BenchmarkEvent

        if (
          event.type === 'task_progress' &&
          event.taskId &&
          event.taskLabel &&
          event.taskStatus
        ) {
          setTasks((current) => ({
            ...current,
            [event.taskId!]: {
              taskId: event.taskId!,
              taskLabel: event.taskLabel!,
              taskStatus: event.taskStatus!,
              taskPercent: event.taskPercent ?? 0,
            },
          }))
          return
        }

        if (typeof event.percent === 'number') {
          setProgress(
            Math.max(
              0,
              Math.min(
                100,
                Math.round(event.percent),
              ),
            ),
          )
        }

        if (event.message) {
          setProgressMessage(event.message)
        }

        if (event.examType) {
          setExamType(event.examType)
        }

        if (event.eye) {
          setEye(event.eye)
        }

        if (event.type === 'error') {
          throw new Error(
            event.error ||
              event.message ||
              'Falha durante a extração.',
          )
        }

        if (event.type === 'result') {
          setFields(event.fields ?? [])
          setExtracted(event.extracted ?? 0)
          setTotal(event.total ?? 0)
          setElapsedMs(event.elapsedMs)
          setSupported(event.supported ?? false)
        }
      }

      while (true) {
        const { value, done } =
          await reader.read()

        if (done) break

        buffer += decoder.decode(
          value,
          { stream: true },
        )

        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''

        for (const line of lines) {
          consume(line)
        }
      }

      buffer += decoder.decode()

      if (buffer.trim()) {
        consume(buffer)
      }
    } catch (runError) {
      setError(
        runError instanceof Error
          ? runError.message
          : 'Não foi possível executar o benchmark.',
      )
    } finally {
      setBusy(false)
    }
  }

  const completed =
    supported !== undefined && !busy

  const coverage =
    total > 0
      ? Math.round((extracted / total) * 100)
      : 0

  const benchmarkTasks = Object.values(tasks)
  const runningTaskCount = benchmarkTasks.filter(
    (task) => task.taskStatus === 'running',
  ).length

  const activeBenchmarkStep =
    progress >= 68 ? 3 : progress >= 25 ? 2 : progress >= 7 ? 1 : 0

  return (
    <div className="space-y-5">
      <section className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
        <span className="text-xs font-bold tracking-[0.13em] text-primary">
          SISTEMA
        </span>

        <h2 className="mb-0 mt-1 font-display text-xl">
          Benchmark OCR
        </h2>

        <p className="mb-0 mt-2 text-sm text-text-secondary">
          O tipo do exame é definido pelo nome do arquivo. O benchmark mede
          somente os campos que o parser correspondente consegue extrair.
        </p>
      </section>

      <section className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
        {!file ? (
          <label
            className={`flex cursor-pointer flex-col items-center justify-center gap-3 rounded-2xl border border-dashed px-6 py-12 text-center transition ${
              dragging
                ? 'border-primary bg-primary-soft'
                : 'border-border-strong bg-surface-muted hover:border-primary'
            }`}
            onDragEnter={(event) => {
              event.preventDefault()
              event.stopPropagation()
              setDragging(true)
            }}
            onDragOver={(event) => {
              event.preventDefault()
              event.stopPropagation()
              event.dataTransfer.dropEffect = 'copy'
              setDragging(true)
            }}
            onDragLeave={(event) => {
              event.preventDefault()
              event.stopPropagation()

              if (event.currentTarget === event.target) {
                setDragging(false)
              }
            }}
            onDrop={(event) => {
              event.preventDefault()
              event.stopPropagation()
              setDragging(false)

              selectBenchmarkFile(
                event.dataTransfer.files?.[0] ?? null,
              )
            }}
          >
            <span className="grid h-12 w-12 place-items-center rounded-xl bg-primary-soft text-primary">
              <UploadCloud size={23} />
            </span>

            <div>
              <strong className="block text-sm text-text-primary">
                {dragging
                  ? 'Solte o exame aqui'
                  : 'Arraste ou selecione um exame'}
              </strong>

              <span className="mt-1 block text-xs text-text-muted">
                Um arquivo por execução
              </span>
            </div>

            <input
              className="hidden"
              onChange={(event) => {
                selectBenchmarkFile(
                  event.target.files?.[0] ?? null,
                )
              }}
              type="file"
            />
          </label>
        ) : (
          <>
            <div className="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-border bg-surface-muted p-4">
              <div className="flex min-w-0 items-center gap-3">
                <span className="grid h-10 w-10 flex-none place-items-center rounded-xl bg-primary-soft text-primary">
                  <FileSearch size={19} />
                </span>

                <div className="min-w-0">
                  <strong
                    className="block truncate text-sm text-text-primary"
                    title={file.name}
                  >
                    {file.name}
                  </strong>

                  <span className="mt-1 block text-xs text-text-muted">
                    {examType || '—'}
                    {eye ? ` • ${eye}` : ''}
                    {' • '}
                    {(file.size / 1024 / 1024).toFixed(2)} MB
                  </span>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <button
                  aria-label="Limpar arquivo selecionado"
                  className="grid h-9 w-9 place-items-center rounded-xl border border-danger/30 text-danger transition hover:border-danger hover:bg-danger-soft disabled:cursor-not-allowed disabled:opacity-50"
                  disabled={busy}
                  onClick={clearBenchmarkFile}
                  title="Limpar arquivo"
                  type="button"
                >
                  <Trash2 size={16} />
                </button>

                <button
                  className="rounded-xl border-0 bg-primary px-4 py-2 text-xs font-bold text-white disabled:opacity-50"
                  disabled={busy}
                  onClick={() => void runBenchmark()}
                  type="button"
                >
                  {busy
                    ? 'Extraindo…'
                    : 'Executar benchmark'}
                </button>
              </div>
            </div>

            {(busy || completed) && (
              <div className="mt-6">
                <div className="flex items-center justify-between gap-4 text-xs">
                  <strong className="text-primary">
                    {progressMessage ||
                      'Preparando extração'}
                  </strong>

                  <span className="font-mono text-text-secondary">
                    {progress}%
                  </span>
                </div>

                <div
                  aria-valuemax={100}
                  aria-valuemin={0}
                  aria-valuenow={progress}
                  className="mt-2 h-2.5 overflow-hidden rounded-full bg-surface-muted"
                  role="progressbar"
                >
                  <div
                    className="h-full rounded-full bg-primary transition-[width] duration-500"
                    style={{
                      width: `${progress}%`,
                    }}
                  />
                </div>
              </div>
            )}

            {busy && (
              <div className="mt-5 flex items-center gap-3 rounded-xl border border-primary-border bg-primary-soft p-4">
                <LoaderCircle
                  className="animate-spin text-primary"
                  size={20}
                />

                <div>
                  <strong className="block text-sm text-text-primary">
                    {examType}
                    {eye ? ` ${eye}` : ''}
                  </strong>

                  <span className="text-xs text-text-muted">
                    {progressMessage}
                  </span>
                </div>
              </div>
            )}

            {!error && (busy || benchmarkTasks.length > 0) && (
              <div className="mt-4 rounded-xl border border-border bg-surface-muted p-4">
                <div className="overflow-x-auto pb-1">
                  <div className="min-w-[640px]">
                    <div className="flex items-stretch">
                      {benchmarkSteps.map((step, index) => {
                        const stepCompleted = completed || index < activeBenchmarkStep
                        const stepActive = !completed && index === activeBenchmarkStep

                        return (
                          <div
                            className={`relative flex min-w-[160px] flex-1 flex-col items-center rounded-xl border px-3 py-3 transition-colors ${
                              stepActive
                                ? 'border-primary bg-primary-soft'
                                : 'border-transparent'
                            }`}
                            key={step}
                          >
                            {index < benchmarkSteps.length - 1 && (
                              <span
                                className={`pointer-events-none absolute left-1/2 top-9 z-10 h-0.5 w-full ${
                                  index < activeBenchmarkStep || completed
                                    ? 'bg-success'
                                    : 'bg-border-strong'
                                }`}
                              />
                            )}

                            <span
                              className={`relative z-20 grid h-12 w-12 flex-none place-items-center rounded-full text-sm font-bold transition-all ${
                                stepCompleted
                                  ? 'bg-success-soft text-success'
                                  : stepActive
                                    ? 'bg-primary text-surface ring-4 ring-primary-soft'
                                    : 'bg-surface text-text-muted ring-1 ring-border'
                              }`}
                            >
                              {stepCompleted ? (
                                <Check size={20} strokeWidth={3} />
                              ) : stepActive ? (
                                <LoaderCircle className="animate-spin" size={20} />
                              ) : (
                                index + 1
                              )}
                            </span>

                            <span
                              className={`relative z-20 mt-2 whitespace-nowrap text-sm font-medium ${
                                stepCompleted || stepActive
                                  ? 'text-text-primary'
                                  : 'text-text-muted'
                              }`}
                            >
                              {step}
                            </span>
                          </div>
                        )
                      })}
                    </div>

                    <div className="mt-3 flex items-center justify-between gap-3">
                      <span className="rounded-full bg-primary-soft px-2.5 py-1 text-[11px] font-medium text-primary">
                        {completed
                          ? 'Extração concluída'
                          : benchmarkSteps[activeBenchmarkStep]}
                      </span>

                      <span className="font-mono text-[11px] text-text-muted">
                        {completed ? '100%' : `${progress}%`}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="flex items-center justify-between gap-3">
                  <strong className="text-sm text-text-primary">
                    Atividades da extração
                  </strong>

                  <span className="text-xs text-text-muted">
                    {busy
                      ? runningTaskCount > 0
                        ? `${runningTaskCount} em execução`
                        : 'Finalizando'
                      : 'Concluído'}
                  </span>
                </div>

                <div className="mt-3 max-h-80 space-y-2 overflow-y-auto pr-1">
                  {benchmarkTasks.map((task) => (
                    <div
                      className="flex items-center gap-3 rounded-lg border border-border bg-surface px-3 py-2.5"
                      key={task.taskId}
                    >
                      {task.taskStatus === 'completed' ? (
                        <span className="grid h-6 w-6 flex-none place-items-center rounded-full bg-success-soft text-success">
                          <Check size={14} strokeWidth={3} />
                        </span>
                      ) : task.taskStatus === 'failed' ? (
                        <span className="grid h-6 w-6 flex-none place-items-center rounded-full bg-danger-soft text-danger">
                          <X size={14} strokeWidth={3} />
                        </span>
                      ) : (
                        <span className="grid h-6 w-6 flex-none place-items-center rounded-full bg-warning-soft text-warning">
                          <LoaderCircle className="animate-spin" size={14} />
                        </span>
                      )}

                      <span className="min-w-0 flex-1 text-xs text-text-secondary">
                        {task.taskLabel}
                      </span>

                      <span
                        className={`font-mono text-[11px] ${
                          task.taskStatus === 'completed'
                            ? 'text-success'
                            : task.taskStatus === 'failed'
                              ? 'text-danger'
                              : 'text-warning'
                        }`}
                      >
                        {task.taskStatus === 'completed'
                          ? 'Concluído'
                          : task.taskStatus === 'failed'
                            ? 'Falhou'
                            : `${task.taskPercent}%`}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {completed && supported && (
              <>
                <div className="mt-6 grid gap-3 sm:grid-cols-3">
                  <div className="rounded-xl border border-border bg-surface-muted p-4">
                    <span className="text-xs font-semibold text-text-muted">
                      Campos extraídos
                    </span>

                    <strong className="mt-2 block font-display text-2xl text-text-primary">
                      {extracted}/{total}
                    </strong>
                  </div>

                  <div className="rounded-xl border border-border bg-surface-muted p-4">
                    <span className="text-xs font-semibold text-text-muted">
                      Cobertura
                    </span>

                    <strong className="mt-2 block font-display text-2xl text-text-primary">
                      {coverage}%
                    </strong>
                  </div>

                  <div className="rounded-xl border border-border bg-surface-muted p-4">
                    <span className="text-xs font-semibold text-text-muted">
                      Tempo
                    </span>

                    <strong className="mt-2 block font-display text-2xl text-text-primary">
                      {duration(elapsedMs)}
                    </strong>
                  </div>
                </div>

                <div className="mt-6">
                  <div className="mb-3 flex items-center justify-between gap-3">
                    <h3 className="m-0 font-display text-lg">
                      Campos
                    </h3>

                    <span className="text-xs text-text-muted">
                      Valores clínicos ocultados neste benchmark
                    </span>
                  </div>

                  <div className="grid gap-2 sm:grid-cols-2">
                    {fields.map((field) => (
                      <div
                        className="flex items-center gap-3 rounded-xl border border-border bg-surface-muted px-4 py-3"
                        key={field.key}
                      >
                        {field.found ? (
                          <span className="grid h-6 w-6 flex-none place-items-center rounded-full bg-success-soft text-success">
                            <Check
                              size={15}
                              strokeWidth={3}
                            />
                          </span>
                        ) : (
                          <span className="grid h-6 w-6 flex-none place-items-center rounded-full bg-danger-soft text-danger">
                            <X
                              size={15}
                              strokeWidth={3}
                            />
                          </span>
                        )}

                        <span className="min-w-0 text-sm font-medium text-text-primary">
                          <span className="block">{field.label}</span>
                          {field.found && field.value && (
                            <strong className="mt-1 block text-sm text-primary">
                              {field.value}
                            </strong>
                          )}
                          {field.unit && <span className="mt-1 block text-xs text-text-muted">{field.unit}</span>}
                          {!field.found && <span className="mt-1 block text-xs text-danger">Não extraído</span>}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              </>
            )}

            {completed && supported === false && (
              <div className="mt-5 rounded-xl border border-warning/30 bg-warning-soft p-4">
                <strong className="block text-sm text-text-primary">
                  {examType}
                  {eye ? ` ${eye}` : ''}
                </strong>

                <span className="mt-1 block text-xs text-text-secondary">
                  Tipo identificado pelo filename, mas o benchmark de campos
                  desse extrator ainda não foi configurado.
                </span>
              </div>
            )}

            {error && (
              <div className="mt-5 rounded-xl border border-danger/30 bg-danger-soft p-4 text-sm text-danger">
                {error}
              </div>
            )}
          </>
        )}
      </section>
      <footer className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
        <button
          aria-controls="benchmark-exam-details"
          aria-expanded={showExamDetails}
          className="flex w-full items-center justify-between gap-4 text-left"
          onClick={() => setShowExamDetails((current) => !current)}
          type="button"
        >
          <span>
            <span className="block text-xs font-bold tracking-[0.13em] text-primary">
              REFERÊNCIA DO BENCHMARK
            </span>

            <span className="mt-1 block font-display text-lg text-text-primary">
              Exames e campos procurados
            </span>

            <span className="mt-1 block text-sm font-normal text-text-secondary">
              Consulte quais informações são avaliadas em cada tipo de exame.
            </span>
          </span>

          <ChevronDown
            aria-hidden="true"
            className={`flex-none text-primary transition-transform ${showExamDetails ? 'rotate-180' : ''}`}
            size={20}
          />
        </button>

        {showExamDetails && (
          <div
            className="mt-6 border-t border-border pt-5"
            id="benchmark-exam-details"
          >
            <p className="mb-4 text-sm text-text-secondary">
              O resultado mede apenas os campos listados abaixo. Um campo é
              considerado extraído quando o parser encontra um valor válido
              para ele no documento.
            </p>

            <div className="grid gap-3 md:grid-cols-2">
              {Object.values(examContracts).map((contract) => (
                <article
                  className="rounded-xl border border-border bg-surface-muted p-4"
                  key={contract.key}
                >
                  <h3 className="m-0 text-sm font-bold text-text-primary">
                    {contract.label}
                  </h3>

                  <ul className="mt-3 flex flex-wrap gap-2 p-0">
                    {contract.fields.map((field) => (
                      <li
                        className="list-none rounded-lg border border-border bg-surface px-2.5 py-1.5 text-xs text-text-secondary"
                        key={field.key}
                      >
                        {field.label}
                      </li>
                    ))}
                  </ul>
                </article>
              ))}
            </div>
          </div>
        )}
      </footer>
    </div>
  )
}
