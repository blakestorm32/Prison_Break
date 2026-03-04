package escape

import (
	"fmt"

	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

type RouteRequirement struct {
	Label string `json:"label"`
	Met   bool   `json:"met"`
}

type RouteEvaluation struct {
	Route         model.EscapeRouteType `json:"route"`
	RouteLabel    string                `json:"route_label"`
	CanAttempt    bool                  `json:"can_attempt"`
	FailureReason string                `json:"failure_reason,omitempty"`
	Requirements  []RouteRequirement    `json:"requirements,omitempty"`
}

func KnownRoutes() []model.EscapeRouteType {
	return []model.EscapeRouteType{
		model.EscapeRouteCourtyardDig,
		model.EscapeRouteBadgeEscape,
		model.EscapeRoutePowerOutEscape,
		model.EscapeRouteLadderEscape,
		model.EscapeRouteRoofHelicopter,
	}
}

func IsKnownRoute(route model.EscapeRouteType) bool {
	for _, candidate := range KnownRoutes() {
		if candidate == route {
			return true
		}
	}
	return false
}

func EvaluateAllRoutes(player model.PlayerState, mapState model.MapState) []RouteEvaluation {
	routes := KnownRoutes()
	out := make([]RouteEvaluation, 0, len(routes))
	for _, route := range routes {
		out = append(out, EvaluateRoute(route, player, mapState))
	}
	return out
}

func EvaluateRoute(route model.EscapeRouteType, player model.PlayerState, mapState model.MapState) RouteEvaluation {
	eval := RouteEvaluation{
		Route:      route,
		RouteLabel: RouteLabel(route),
	}
	if !IsKnownRoute(route) {
		eval.FailureReason = "Unknown escape route."
		return eval
	}

	eval.Requirements = append(eval.Requirements,
		RouteRequirement{Label: "Alive", Met: player.Alive},
		RouteRequirement{Label: "Prisoner faction", Met: gamemap.IsPrisonerPlayer(player)},
	)

	switch route {
	case model.EscapeRouteCourtyardDig:
		eval.Requirements = append(eval.Requirements,
			RouteRequirement{Label: "Room: courtyard", Met: player.CurrentRoomID == gamemap.RoomCourtyard},
			RouteRequirement{Label: "Item: shovel x1", Met: items.HasItem(player, model.ItemShovel, 1)},
		)
	case model.EscapeRouteBadgeEscape:
		eval.Requirements = append(eval.Requirements,
			RouteRequirement{Label: "Room: main corridor", Met: player.CurrentRoomID == gamemap.RoomCorridorMain},
			RouteRequirement{Label: "Item: badge x1", Met: items.HasItem(player, model.ItemBadge, 1)},
		)
	case model.EscapeRoutePowerOutEscape:
		eval.Requirements = append(eval.Requirements,
			RouteRequirement{Label: "Room: power room", Met: player.CurrentRoomID == gamemap.RoomPowerRoom},
			RouteRequirement{Label: "Power state: OFF", Met: !mapState.PowerOn},
		)
	case model.EscapeRouteLadderEscape:
		eval.Requirements = append(eval.Requirements,
			RouteRequirement{Label: "Room: courtyard", Met: player.CurrentRoomID == gamemap.RoomCourtyard},
			RouteRequirement{Label: "Item: ladder x2", Met: items.HasItem(player, model.ItemLadder, 2)},
		)
	case model.EscapeRouteRoofHelicopter:
		eval.Requirements = append(eval.Requirements,
			RouteRequirement{Label: "Room: roof lookout", Met: player.CurrentRoomID == gamemap.RoomRoofLookout},
			RouteRequirement{Label: "Item: keys x1", Met: items.HasItem(player, model.ItemKeys, 1)},
		)
	}

	for _, requirement := range eval.Requirements {
		if requirement.Met {
			continue
		}
		eval.FailureReason = fmt.Sprintf("Missing requirement: %s.", requirement.Label)
		return eval
	}

	eval.CanAttempt = true
	return eval
}

func CanAttemptRoute(route model.EscapeRouteType, player model.PlayerState, mapState model.MapState) bool {
	return EvaluateRoute(route, player, mapState).CanAttempt
}

func RouteLabel(route model.EscapeRouteType) string {
	switch route {
	case model.EscapeRouteCourtyardDig:
		return "Courtyard Dig"
	case model.EscapeRouteBadgeEscape:
		return "Badge Escape"
	case model.EscapeRoutePowerOutEscape:
		return "Power-Out Escape"
	case model.EscapeRouteLadderEscape:
		return "Ladder Escape"
	case model.EscapeRouteRoofHelicopter:
		return "Roof Helicopter"
	default:
		return "Unknown Route"
	}
}
