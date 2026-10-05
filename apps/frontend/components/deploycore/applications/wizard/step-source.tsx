'use client'

import { useMemo } from 'react'
import {
  Controller,
  type Control,
  type FieldErrors,
  type UseFormRegister,
  type UseFormWatch,
} from 'react-hook-form'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldLegend, FieldSet } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { GIT_PROVIDERS_PATH } from '@/lib/github/callback'
import {
  connectionListMessage,
  GIT_CONNECTION_NONE,
  GIT_CONNECTION_NONE_ACTIVE,
  GIT_CONNECTION_NONE_HINT,
  GIT_REPOSITORY_NONE,
  GIT_REPOSITORY_NONE_HINT,
  repositoryListMessage,
  type GitSourceLoadPhase,
  type WizardConnectionOption,
  type WizardRepositoryOption,
} from '@/lib/applications/git-source-wizard'
import { cn } from '@/lib/utils'
import {
  sourceTypeLabel,
  type CreateApplicationValues,
  type RepositorySourceMode,
  type SourceType,
} from '@/lib/validations/application'
import { Container, FileStack, GitBranch } from 'lucide-react'

const OPTIONS: { value: SourceType; icon: typeof GitBranch; description: string }[] = [
  {
    value: 'git',
    icon: GitBranch,
    description: 'Build from a Git repository with a Dockerfile.',
  },
  {
    value: 'docker-image',
    icon: Container,
    description: 'Deploy an existing image from a registry.',
  },
  {
    value: 'docker-compose',
    icon: FileStack,
    description: 'Deploy services defined in a Compose file.',
  },
]

interface StepSourceProps {
  watch: UseFormWatch<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
  onSourceType: (sourceType: SourceType) => void
}

export function StepSource({ watch, errors, onSourceType }: StepSourceProps) {
  const selected = watch('sourceType')

  return (
    <FieldGroup>
      <Field data-invalid={Boolean(errors.sourceType) || undefined}>
        <FieldLabel>Source</FieldLabel>
        <FieldDescription>Choose how this application will be built and deployed.</FieldDescription>
        <div className="grid gap-2">
          {OPTIONS.map((option) => {
            const Icon = option.icon
            const active = selected === option.value
            return (
              <button
                key={option.value}
                type="button"
                className={cn(
                  'flex items-start gap-3 rounded-lg border px-3 py-2.5 text-left transition-colors',
                  active
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:border-primary/40 hover:bg-muted/30',
                )}
                onClick={() => onSourceType(option.value)}
                aria-pressed={active}
              >
                <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
                <span className="min-w-0">
                  <span className="block text-sm font-medium">{sourceTypeLabel(option.value)}</span>
                  <span className="block text-xs text-muted-foreground">{option.description}</span>
                </span>
              </button>
            )
          })}
        </div>
        <FieldError>{errors.sourceType?.message}</FieldError>
      </Field>
    </FieldGroup>
  )
}

export interface GitSourceStepControls {
  connections: WizardConnectionOption[]
  repositories: WizardRepositoryOption[]
  connectionPhase: GitSourceLoadPhase
  repositoryPhase: GitSourceLoadPhase
  selectRepositorySource: (mode: RepositorySourceMode) => void
  selectConnection: (connectionId: string) => void
  selectRepository: (repositoryId: string) => void
}

interface StepSourceConfigProps {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  watch: UseFormWatch<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
  gitSource: GitSourceStepControls
}

export function StepSourceConfig({ register, control, watch, errors, gitSource }: StepSourceConfigProps) {
  const sourceType = watch('sourceType')
  const repositorySource = watch('repositorySource')
  const gitConnectionId = watch('gitConnectionId')

  return (
    <FieldGroup>
      {sourceType === 'git' && (
        <GitRepositoryFields
          register={register}
          control={control}
          errors={errors}
          repositorySource={repositorySource}
          gitConnectionId={gitConnectionId}
          gitSource={gitSource}
        />
      )}

      {sourceType === 'docker-image' && (
        <>
          <Field data-invalid={Boolean(errors.image) || undefined}>
            <FieldLabel htmlFor="wizard-image">Image</FieldLabel>
            <Input
              id="wizard-image"
              className="font-mono"
              placeholder="registry.example.com/app"
              aria-invalid={Boolean(errors.image)}
              {...register('image')}
            />
            <FieldError>{errors.image?.message}</FieldError>
          </Field>
          <Field data-invalid={Boolean(errors.imageTag) || undefined}>
            <FieldLabel htmlFor="wizard-image-tag">Tag</FieldLabel>
            <Input
              id="wizard-image-tag"
              className="font-mono"
              aria-invalid={Boolean(errors.imageTag)}
              {...register('imageTag')}
            />
            <FieldError>{errors.imageTag?.message}</FieldError>
          </Field>
        </>
      )}

      {sourceType === 'docker-compose' && (
        <>
          <Field data-invalid={Boolean(errors.repository) || undefined}>
            <FieldLabel htmlFor="wizard-repository">Repository</FieldLabel>
            <Input
              id="wizard-repository"
              placeholder="github.com/org/app"
              aria-invalid={Boolean(errors.repository)}
              {...register('repository')}
            />
            <FieldError>{errors.repository?.message}</FieldError>
          </Field>
          <Field data-invalid={Boolean(errors.branch) || undefined}>
            <FieldLabel htmlFor="wizard-branch">Branch</FieldLabel>
            <Input id="wizard-branch" aria-invalid={Boolean(errors.branch)} {...register('branch')} />
            <FieldError>{errors.branch?.message}</FieldError>
          </Field>
          <Field data-invalid={Boolean(errors.composeFile) || undefined}>
            <FieldLabel htmlFor="wizard-compose">Compose file</FieldLabel>
            <Input
              id="wizard-compose"
              className="font-mono"
              aria-invalid={Boolean(errors.composeFile)}
              {...register('composeFile')}
            />
            <FieldError>{errors.composeFile?.message}</FieldError>
          </Field>
        </>
      )}
    </FieldGroup>
  )
}

function GitRepositoryFields({
  register,
  control,
  errors,
  repositorySource,
  gitConnectionId,
  gitSource,
}: {
  register: UseFormRegister<CreateApplicationValues>
  control: Control<CreateApplicationValues>
  errors: FieldErrors<CreateApplicationValues>
  repositorySource: RepositorySourceMode
  gitConnectionId: string
  gitSource: GitSourceStepControls
}) {
  const selectableCount = gitSource.connections.filter((connection) => connection.selectable).length
  const connectionMessage = connectionListMessage({
    phase: gitSource.connectionPhase,
    visibleCount: gitSource.connections.length,
    selectableCount,
  })
  const repositoryMessage = repositoryListMessage({
    connectionSelected: Boolean(gitConnectionId),
    phase: gitSource.repositoryPhase,
    repositoryCount: gitSource.repositories.length,
  })
  const connectionItems = useMemo(() => {
    const items: Record<string, string> = {}
    for (const connection of gitSource.connections) items[connection.id] = connection.label
    return items
  }, [gitSource.connections])
  const repositoryItems = useMemo(() => {
    const items: Record<string, string> = {}
    for (const repository of gitSource.repositories) items[repository.id] = repository.label
    return items
  }, [gitSource.repositories])
  const showConnectionLink = connectionMessage === GIT_CONNECTION_NONE || connectionMessage === GIT_CONNECTION_NONE_ACTIVE

  return (
    <>
      <FieldSet>
        <FieldLegend variant="label">Repository source</FieldLegend>
        <div className="grid gap-2 sm:grid-cols-2">
          <RepositorySourceChoice
            id="wizard-repository-source-connected"
            value="connected"
            checked={repositorySource === 'connected'}
            title="Connected repository"
            description="Choose a repository from a GitHub connection."
            onSelect={gitSource.selectRepositorySource}
          />
          <RepositorySourceChoice
            id="wizard-repository-source-public"
            value="public"
            checked={repositorySource === 'public'}
            title="Public Git URL"
            description="Enter a public repository URL and branch."
            onSelect={gitSource.selectRepositorySource}
          />
        </div>
      </FieldSet>

      {repositorySource === 'connected' ? (
        <>
          <Field data-invalid={Boolean(errors.gitConnectionId) || undefined}>
            <FieldLabel htmlFor="wizard-git-connection">Git connection</FieldLabel>
            <Controller
              name="gitConnectionId"
              control={control}
              render={({ field }) => (
                <Select
                  items={connectionItems}
                  value={field.value || null}
                  disabled={gitSource.connections.length === 0}
                  onValueChange={(value) => {
                    if (!value) return
                    gitSource.selectConnection(value)
                  }}
                >
                  <SelectTrigger
                    id="wizard-git-connection"
                    className="w-full"
                    aria-invalid={Boolean(errors.gitConnectionId)}
                  >
                    <SelectValue
                      placeholder={
                        gitSource.connectionPhase === 'loading' ? 'Loading Git connections…' : 'Select a Git connection'
                      }
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {gitSource.connections.map((connection) => (
                      <SelectItem
                        key={connection.id}
                        value={connection.id}
                        label={connection.label}
                        disabled={!connection.selectable}
                      >
                        {connection.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            {connectionMessage ? (
              <FieldDescription role={gitSource.connectionPhase === 'error' ? 'alert' : 'status'}>
                {connectionMessage}
                {showConnectionLink ? (
                  <>
                    {' '}
                    {GIT_CONNECTION_NONE_HINT}{' '}
                    <a href={GIT_PROVIDERS_PATH} className="text-primary underline-offset-4 hover:underline">
                      Open Git Providers
                    </a>
                    .
                  </>
                ) : null}
              </FieldDescription>
            ) : null}
            <FieldError>{errors.gitConnectionId?.message}</FieldError>
          </Field>

          <Field data-invalid={Boolean(errors.repositoryId) || undefined}>
            <FieldLabel htmlFor="wizard-git-repository">Repository</FieldLabel>
            <Controller
              name="repositoryId"
              control={control}
              render={({ field }) => (
                <Select
                  items={repositoryItems}
                  value={field.value || null}
                  disabled={!gitConnectionId || gitSource.repositoryPhase === 'loading'}
                  onValueChange={(value) => {
                    if (!value) return
                    gitSource.selectRepository(value)
                  }}
                >
                  <SelectTrigger
                    id="wizard-git-repository"
                    className="w-full"
                    aria-invalid={Boolean(errors.repositoryId)}
                  >
                    <SelectValue placeholder={repositoryPlaceholder(gitConnectionId, gitSource.repositoryPhase)} />
                  </SelectTrigger>
                  <SelectContent>
                    {gitSource.repositories.map((repository) => (
                      <SelectItem
                        key={repository.id}
                        value={repository.id}
                        label={repository.label}
                        disabled={!repository.selectable}
                      >
                        {repository.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            {repositoryMessage ? (
              <FieldDescription role={gitSource.repositoryPhase === 'error' ? 'alert' : 'status'}>
                {repositoryMessage}
                {repositoryMessage === GIT_REPOSITORY_NONE ? (
                  <>
                    {' '}
                    {GIT_REPOSITORY_NONE_HINT}{' '}
                    <a href={GIT_PROVIDERS_PATH} className="text-primary underline-offset-4 hover:underline">
                      Open Git Providers
                    </a>
                    .
                  </>
                ) : null}
              </FieldDescription>
            ) : null}
            <FieldError>{errors.repositoryId?.message}</FieldError>
          </Field>
        </>
      ) : (
        <Field data-invalid={Boolean(errors.repository) || undefined}>
          <FieldLabel htmlFor="wizard-repository">Repository</FieldLabel>
          <Input
            id="wizard-repository"
            placeholder="github.com/org/app"
            aria-invalid={Boolean(errors.repository)}
            {...register('repository')}
          />
          <FieldError>{errors.repository?.message}</FieldError>
        </Field>
      )}

      <Field data-invalid={Boolean(errors.branch) || undefined}>
        <FieldLabel htmlFor="wizard-branch">Branch</FieldLabel>
        <Input id="wizard-branch" aria-invalid={Boolean(errors.branch)} {...register('branch')} />
        <FieldError>{errors.branch?.message}</FieldError>
      </Field>
      <Field data-invalid={Boolean(errors.dockerfile) || undefined}>
        <FieldLabel htmlFor="wizard-dockerfile">Dockerfile</FieldLabel>
        <Input
          id="wizard-dockerfile"
          className="font-mono"
          aria-invalid={Boolean(errors.dockerfile)}
          {...register('dockerfile')}
        />
        <FieldError>{errors.dockerfile?.message}</FieldError>
      </Field>
      <Field data-invalid={Boolean(errors.buildContext) || undefined}>
        <FieldLabel htmlFor="wizard-build-context">Build context</FieldLabel>
        <Input
          id="wizard-build-context"
          className="font-mono"
          aria-invalid={Boolean(errors.buildContext)}
          {...register('buildContext')}
        />
        <FieldDescription>Path relative to the repository root.</FieldDescription>
        <FieldError>{errors.buildContext?.message}</FieldError>
      </Field>
    </>
  )
}

function RepositorySourceChoice({
  id,
  value,
  checked,
  title,
  description,
  onSelect,
}: {
  id: string
  value: RepositorySourceMode
  checked: boolean
  title: string
  description: string
  onSelect: (mode: RepositorySourceMode) => void
}) {
  return (
    <label
      htmlFor={id}
      className={cn(
        'flex items-start gap-2 rounded-lg border px-3 py-2.5',
        checked ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/40',
      )}
    >
      <input
        id={id}
        type="radio"
        name="wizard-repository-source"
        value={value}
        checked={checked}
        className="mt-1"
        onChange={() => onSelect(value)}
      />
      <span className="min-w-0">
        <span className="block text-sm font-medium">{title}</span>
        <span className="block text-xs text-muted-foreground">{description}</span>
      </span>
    </label>
  )
}

function repositoryPlaceholder(connectionId: string, phase: GitSourceLoadPhase): string {
  if (!connectionId) return 'Select a connection first'
  if (phase === 'loading') return 'Loading repositories…'
  return 'Select a repository'
}

export type WizardFieldErrors = FieldErrors<CreateApplicationValues>
