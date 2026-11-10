import { useState, type FormEvent } from 'react'
import { answerRequest, type Answer } from '../api'
import type { FormField, PendingRequest, Question, RequestPayload } from '../types'
import { IconButton } from './Icon'

interface RequestCardProps {
  conversationId: string
  request: PendingRequest
}

// RequestCard is answerable only because the service's pending set lists the
// request. the first answer from any browser wins; a later one is refused.
export function RequestCard({ conversationId, request }: RequestCardProps) {
  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)

  const send = async (answer: Answer) => {
    setSending(true)
    setError(null)
    try {
      await answerRequest(conversationId, request.id, answer)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSending(false)
    }
  }

  return (
    <section className="request-card" aria-label={request.kind === 'question' ? 'Question' : 'Approval'}>
      {request.kind === 'question' && request.payload?.fields ? (
        <FormQuestion payload={request.payload} sending={sending} onAnswer={(answers) => void send({ answers })} />
      ) : request.kind === 'question' ? (
        <QuestionForm questions={request.payload?.questions ?? []} sending={sending} onAnswer={(answers) => void send({ answers })} />
      ) : (
        <PermissionForm request={request} sending={sending} onAnswer={(answer) => void send(answer)} />
      )}
      {error && <p className="error">{error}</p>}
    </section>
  )
}

function describeInput(input: Record<string, unknown> | undefined): string {
  if (!input) {
    return ''
  }
  for (const key of ['command', 'file_path', 'path', 'url', 'pattern']) {
    if (typeof input[key] === 'string') {
      return input[key]
    }
  }
  return JSON.stringify(input, null, 2)
}

function PermissionForm({ request, sending, onAnswer }: { request: PendingRequest; sending: boolean; onAnswer: (answer: Answer) => void }) {
  const [message, setMessage] = useState('')
  const payload = request.payload ?? {}
  if (payload.mcp_server) {
    return <McpApproval payload={payload} sending={sending} onAnswer={onAnswer} />
  }
  const detail = payload.title ?? payload.description ?? payload.decision_reason
  return (
    <>
      <h2>Approval: {payload.display_name ?? payload.tool ?? 'tool'}</h2>
      {payload.permission_mode === 'auto' && (
        <p className="muted">Raised in auto mode: the classifier could not decide this call, so it needs your answer.</p>
      )}
      {detail && <p>{detail}</p>}
      <pre>{describeInput(payload.input)}</pre>
      {payload.blocked_path && (
        <p className="muted">
          Path: <code>{payload.blocked_path}</code>
        </p>
      )}
      <div className="inline-form">
        <input placeholder="Reason, sent with a denial" value={message} onChange={(event) => setMessage(event.target.value)} />
        <IconButton icon="check" label="Allow" word="Allow" disabled={sending} onClick={() => onAnswer({ decision: 'allow' })} />
        <IconButton icon="block" label="Deny" word="Deny" disabled={sending} onClick={() => onAnswer({ decision: 'deny', message })} />
      </div>
    </>
  )
}

// McpApproval is an MCP tool approval: the server and tool, what the tool
// does, and the parameters it would be called with. codex carries no reason
// with a denial, so none is asked for. "for this session" appears only where
// the harness offered it, and covers this tool only.
function McpApproval({ payload, sending, onAnswer }: { payload: RequestPayload; sending: boolean; onAnswer: (answer: Answer) => void }) {
  const parameters = payload.parameters ?? []
  return (
    <>
      <h2>Approval: {payload.tool ?? 'MCP tool'}</h2>
      <p className="muted">
        MCP server <code>{payload.mcp_server}</code> wants to run its tool <code>{payload.mcp_tool || 'unknown'}</code>.
      </p>
      {payload.description && <p>{payload.description}</p>}
      {parameters.length > 0 ? (
        <dl className="mcp-parameters">
          {parameters.map((parameter) => (
            <div key={parameter.name}>
              <dt>{parameter.display_name ?? parameter.name}</dt>
              <dd>
                <pre>{typeof parameter.value === 'string' ? parameter.value : JSON.stringify(parameter.value, null, 2)}</pre>
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        payload.input && Object.keys(payload.input).length > 0 && <pre>{JSON.stringify(payload.input, null, 2)}</pre>
      )}
      <div className="actions">
        <IconButton icon="check" label="Allow" word="Allow" disabled={sending} onClick={() => onAnswer({ decision: 'allow' })} />
        {payload.allow_session && (
          <IconButton
            icon="done_all"
            label="Allow this tool for the session"
            word="Allow this tool for the session"
            disabled={sending}
            onClick={() => onAnswer({ decision: 'allow_session' })}
          />
        )}
        <IconButton icon="block" label="Deny" word="Deny" disabled={sending} onClick={() => onAnswer({ decision: 'deny' })} />
      </div>
    </>
  )
}

// FormQuestion is a typed form an MCP server asked for, rendered field by
// field: a choice for an enum, a checkbox for a boolean, a number or text
// input otherwise. every answer travels as text; the adapter types it.
function FormQuestion({ payload, sending, onAnswer }: { payload: RequestPayload; sending: boolean; onAnswer: (answers: Record<string, string>) => void }) {
  const fields = payload.fields ?? []
  const [values, setValues] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {}
    for (const field of fields) {
      if (field.type === 'boolean') {
        initial[field.name] = String(field.default === true)
      } else if (field.default !== undefined && field.default !== null) {
        initial[field.name] = String(field.default)
      }
    }
    return initial
  })
  const set = (name: string, value: string) => setValues((current) => ({ ...current, [name]: value }))
  const complete = fields.every((field) => !field.required || (values[field.name] ?? '').trim() !== '')

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (!complete) {
      return
    }
    const answers: Record<string, string> = {}
    for (const field of fields) {
      answers[field.name] = (values[field.name] ?? '').trim()
    }
    onAnswer(answers)
  }

  return (
    <form onSubmit={onSubmit}>
      <h2>Question{payload.mcp_server && <> from <code>{payload.mcp_server}</code></>}</h2>
      {payload.message && <p>{payload.message}</p>}
      {fields.map((field) => (
        <FormFieldInput key={field.name} field={field} value={values[field.name] ?? ''} onChange={(value) => set(field.name, value)} />
      ))}
      <div className="actions">
        <IconButton type="submit" icon="reply" label="Answer" word="Answer" disabled={sending || !complete} />
      </div>
    </form>
  )
}

function FormFieldInput({ field, value, onChange }: { field: FormField; value: string; onChange: (value: string) => void }) {
  const label = (
    <>
      {field.title ?? field.name}
      {field.required && <span className="muted"> (required)</span>}
      {field.description && <span className="muted"> {field.description}</span>}
    </>
  )
  if (field.type === 'enum') {
    return (
      <fieldset>
        <legend>{label}</legend>
        {(field.options ?? []).map((option) => (
          <label key={option.value} className="option">
            <input type="radio" name={field.name} checked={value === option.value} onChange={() => onChange(option.value)} />
            <span>{option.label}</span>
          </label>
        ))}
      </fieldset>
    )
  }
  if (field.type === 'boolean') {
    return (
      <label className="option">
        <input type="checkbox" checked={value === 'true'} onChange={(event) => onChange(String(event.target.checked))} />
        <span>{label}</span>
      </label>
    )
  }
  return (
    <label className="form-field">
      <span>{label}</span>
      <input
        type={field.type === 'string' ? 'text' : 'number'}
        step={field.type === 'integer' ? 1 : 'any'}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}

function QuestionForm({ questions, sending, onAnswer }: { questions: Question[]; sending: boolean; onAnswer: (answers: Record<string, string>) => void }) {
  const [chosen, setChosen] = useState<Record<string, string[]>>({})
  const [other, setOther] = useState<Record<string, string>>({})

  const toggle = (question: Question, label: string) => {
    setChosen((current) => {
      const picked = current[question.question] ?? []
      if (!question.multiSelect) {
        return { ...current, [question.question]: [label] }
      }
      return {
        ...current,
        [question.question]: picked.includes(label) ? picked.filter((l) => l !== label) : [...picked, label],
      }
    })
  }

  const answers: Record<string, string> = {}
  for (const question of questions) {
    const typed = other[question.question]?.trim()
    const picked = chosen[question.question] ?? []
    const value = typed ? [...picked, typed] : picked
    if (value.length > 0) {
      answers[question.question] = value.join(', ')
    }
  }
  const complete = questions.length > 0 && questions.every((question) => answers[question.question])

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (complete) {
      onAnswer(answers)
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <h2>Question</h2>
      {questions.map((question) => (
        <fieldset key={question.question}>
          <legend>
            {question.header && <span className="tag">{question.header}</span>} {question.question}
          </legend>
          {(question.options ?? []).map((option) => (
            <label key={option.label} className="option">
              <input
                type={question.multiSelect ? 'checkbox' : 'radio'}
                name={question.question}
                checked={(chosen[question.question] ?? []).includes(option.label)}
                onChange={() => toggle(question, option.label)}
              />
              <span>
                {option.label}
                {option.description && <span className="muted"> {option.description}</span>}
              </span>
            </label>
          ))}
          <input
            placeholder="Or write your own answer"
            value={other[question.question] ?? ''}
            onChange={(event) => setOther((current) => ({ ...current, [question.question]: event.target.value }))}
          />
        </fieldset>
      ))}
      <div className="actions">
        <IconButton type="submit" icon="reply" label="Answer" word="Answer" disabled={sending || !complete} />
      </div>
    </form>
  )
}
