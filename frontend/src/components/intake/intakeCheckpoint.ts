export type IntakeFileCheckpointStatus =
  | 'processed'
  | 'saved'
  | 'new'

export interface IntakeFileCheckpointState {
  filename: string
  sha256: string
  status: IntakeFileCheckpointStatus
  examType?: string
  eye?: string
  processedAt?: string
}

interface CheckpointApiFile {
  filename?: unknown
  sha256?: unknown
  status?: unknown
  examType?: unknown
  eye?: unknown
  processedAt?: unknown
}

interface CheckpointApiResponse {
  processorVersion?: unknown
  files?: unknown
}

export function intakeFileCheckpointKey(
  file: Pick<
    File,
    'name' | 'size' | 'lastModified'
  >,
) {
  return [
    file.name,
    file.size,
    file.lastModified,
  ].join('::')
}

function isCheckpointStatus(
  value: unknown,
): value is IntakeFileCheckpointStatus {
  return (
    value === 'processed' ||
    value === 'saved' ||
    value === 'new'
  )
}

async function calculateFileSHA256(
  file: File,
) {
  const bytes =
    await file.arrayBuffer()

  const digest =
    await crypto.subtle.digest(
      'SHA-256',
      bytes,
    )

  return Array.from(
    new Uint8Array(digest),
  )
    .map((byte) =>
      byte
        .toString(16)
        .padStart(2, '0'),
    )
    .join('')
}

export async function inspectIntakeFileCheckpoints(
  apiUrl: string,
  files: File[],
  signal?: AbortSignal,
): Promise<
  Record<
    string,
    IntakeFileCheckpointState
  >
> {
  const hashedFiles: Array<{
    file: File
    sha256: string
  }> = []

  // Sequencial de propósito:
  // evita manter vários ArrayBuffers grandes
  // simultaneamente em memória.
  for (const file of files) {
    if (signal?.aborted) {
      throw new DOMException(
        'Checkpoint inspection aborted',
        'AbortError',
      )
    }

    const sha256 =
      await calculateFileSHA256(file)

    hashedFiles.push({
      file,
      sha256,
    })
  }

  const response =
    await fetch(
      `${apiUrl}/api/file-checkpoints/check`,
      {
        method: 'POST',
        headers: {
          'Content-Type':
            'application/json',
        },
        signal,
        body: JSON.stringify({
          files: hashedFiles.map(
            ({ file, sha256 }) => ({
              filename: file.name,
              sha256,
            }),
          ),
        }),
      },
    )

  const responseText =
    await response.text()

  let payload:
    | CheckpointApiResponse
    | undefined

  try {
    payload =
      responseText
        ? JSON.parse(responseText)
        : undefined
  } catch {
    throw new Error(
      `A consulta de checkpoints retornou uma resposta inválida (HTTP ${response.status}).`,
    )
  }

  if (!response.ok) {
    const errorMessage =
      payload &&
      typeof payload === 'object' &&
      'error' in payload &&
      typeof payload.error === 'string'
        ? payload.error
        : `Não foi possível consultar os arquivos já processados (HTTP ${response.status}).`

    throw new Error(errorMessage)
  }

  if (
    !payload ||
    !Array.isArray(payload.files)
  ) {
    throw new Error(
      'A consulta de checkpoints não retornou files[].',
    )
  }

  if (
    payload.files.length !==
    hashedFiles.length
  ) {
    throw new Error(
      'A consulta de checkpoints retornou uma quantidade inesperada de arquivos.',
    )
  }

  const responseFiles =
    payload.files as CheckpointApiFile[]

  const states: Record<
    string,
    IntakeFileCheckpointState
  > = {}

  hashedFiles.forEach(
    ({ file, sha256 }, index) => {
      const result =
        responseFiles[index]

      if (
        !result ||
        result.sha256 !== sha256 ||
        !isCheckpointStatus(
          result.status,
        )
      ) {
        throw new Error(
          `O backend retornou um checkpoint inconsistente para ${file.name}.`,
        )
      }

      states[
        intakeFileCheckpointKey(file)
      ] = {
        filename: file.name,
        sha256,
        status: result.status,
        examType:
          typeof result.examType ===
          'string'
            ? result.examType
            : undefined,
        eye:
          typeof result.eye ===
          'string'
            ? result.eye
            : undefined,
        processedAt:
          typeof result.processedAt ===
          'string'
            ? result.processedAt
            : undefined,
      }
    },
  )

  return states
}
