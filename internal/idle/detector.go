package idle

import (
	"context"
	"fmt"
	"strings"
)

// PlayerDetector consulta quantos jogadores estão atualmente
// conectados a um servidor.
//
// Cada protocolo ou jogo implementa esta interface separadamente.
type PlayerDetector interface {
	DetectorType() Detector

	PlayerCount(
		ctx context.Context,
		server Server,
	) (int, error)
}

// DetectorRegistry mantém os detectores disponíveis para o
// motor genérico de Idle.
type DetectorRegistry struct {
	detectors map[Detector]PlayerDetector
}

// NewDetectorRegistry cria um registro com os detectores informados.
//
// O registro rejeita:
//   - detectores nulos;
//   - detectores sem tipo;
//   - detectores desconhecidos;
//   - dois detectores do mesmo tipo.
func NewDetectorRegistry(
	detectors ...PlayerDetector,
) (*DetectorRegistry, error) {
	registry := &DetectorRegistry{
		detectors: make(
			map[Detector]PlayerDetector,
			len(detectors),
		),
	}

	for index, detector := range detectors {
		if detector == nil {
			return nil, fmt.Errorf(
				"o detector de índice %d é nulo",
				index,
			)
		}

		detectorType := Detector(
			strings.TrimSpace(
				string(
					detector.DetectorType(),
				),
			),
		)

		if detectorType == "" {
			return nil, fmt.Errorf(
				"o detector de índice %d não informou seu tipo",
				index,
			)
		}

		if !isKnownDetector(detectorType) {
			return nil, fmt.Errorf(
				"o detector %q não é reconhecido",
				detectorType,
			)
		}

		if _, exists := registry.detectors[detectorType]; exists {
			return nil, fmt.Errorf(
				"o detector %q foi registrado mais de uma vez",
				detectorType,
			)
		}

		registry.detectors[detectorType] = detector
	}

	return registry, nil
}

// NewDefaultDetectorRegistry cria o registro usado pelo AmpControl.
//
// O registro sempre inclui Palworld RCON e recebe detectores que dependem
// de serviços construídos em tempo de execução, como a API do AMP.
func NewDefaultDetectorRegistry(
	additional ...PlayerDetector,
) (
	*DetectorRegistry,
	error,
) {
	detectors := []PlayerDetector{
		NewPalworldRCONDetector(),
	}

	detectors = append(detectors, additional...)

	return NewDetectorRegistry(detectors...)
}

// Supports informa se existe um detector registrado para o tipo.
func (r *DetectorRegistry) Supports(
	detectorType Detector,
) bool {
	if r == nil {
		return false
	}

	detectorType = Detector(
		strings.TrimSpace(
			string(detectorType),
		),
	)

	_, exists := r.detectors[detectorType]

	return exists
}

// PlayerCount seleciona o detector configurado para o servidor
// e consulta a quantidade atual de jogadores.
//
// Servidores desabilitados são rejeitados para impedir que uma
// configuração ainda não aprovada seja monitorada acidentalmente.
func (r *DetectorRegistry) PlayerCount(
	ctx context.Context,
	server Server,
) (int, error) {
	if r == nil {
		return 0, fmt.Errorf(
			"o registro de detectores não foi inicializado",
		)
	}

	if ctx == nil {
		return 0, fmt.Errorf(
			"o contexto da consulta de jogadores é nulo",
		)
	}

	instance := strings.TrimSpace(
		server.Instance,
	)

	if instance == "" {
		return 0, fmt.Errorf(
			"o nome da instância AMP não foi informado",
		)
	}

	if !server.Enabled {
		return 0, fmt.Errorf(
			"o monitor de Idle da instância %s está desabilitado",
			instance,
		)
	}

	detectorType := Detector(
		strings.TrimSpace(
			string(server.Detector),
		),
	)

	if detectorType == "" {
		return 0, fmt.Errorf(
			"a instância %s não possui detector configurado",
			instance,
		)
	}

	playerCount, err := r.playerCountForDetector(
		ctx,
		server,
		detectorType,
	)

	fallbackType := Detector(
		strings.TrimSpace(string(server.FallbackDetector)),
	)

	// A API do AMP é a fonte principal. Quando ela falha, o detector
	// específico assume. Quando a API informa zero, o fallback confirma
	// esse zero antes que o cronômetro de Idle avance.
	if fallbackType != "" &&
		(err != nil || playerCount == 0) {
		fallbackCount, fallbackErr := r.playerCountForDetector(
			ctx,
			server,
			fallbackType,
		)
		if fallbackErr != nil {
			if err != nil {
				return 0, fmt.Errorf(
					"detector primário %q falhou (%v) e o fallback %q também falhou: %w",
					detectorType,
					err,
					fallbackType,
					fallbackErr,
				)
			}

			return 0, fmt.Errorf(
				"o detector %q informou zero, mas o fallback %q não pôde confirmar: %w",
				detectorType,
				fallbackType,
				fallbackErr,
			)
		}

		return fallbackCount, nil
	}

	if err != nil {
		return 0, err
	}

	return playerCount, nil
}

func (r *DetectorRegistry) playerCountForDetector(
	ctx context.Context,
	server Server,
	detectorType Detector,
) (int, error) {
	instance := strings.TrimSpace(server.Instance)

	detector, exists := r.detectors[detectorType]
	if !exists {
		return 0, fmt.Errorf(
			"o detector %q da instância %s não está registrado",
			detectorType,
			instance,
		)
	}

	playerCount, err := detector.PlayerCount(
		ctx,
		server,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"o detector %q falhou na instância %s: %w",
			detectorType,
			instance,
			err,
		)
	}

	if playerCount < 0 {
		return 0, fmt.Errorf(
			"o detector %q retornou uma quantidade negativa de jogadores para a instância %s: %d",
			detectorType,
			instance,
			playerCount,
		)
	}

	return playerCount, nil
}
