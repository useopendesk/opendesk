defmodule RealtimeWeb.UserSocket do
  @moduledoc """
  The agents' socket at `/socket` (`contracts/realtime.md`). Connect with
  `?vsn=2.0.0&token=<jwt>`; only `ws` tokens (users and API keys) are accepted,
  and browser origins must be in `CHECK_ORIGINS`. The chat widget uses
  `RealtimeWeb.WidgetSocket` instead.

  The token is verified once at connect and then forgotten: only its claims are
  kept in assigns. Connection logging is off so the query string, which holds
  the token, is never written anywhere.
  """

  use Phoenix.Socket, log: false

  channel "workspace:*", RealtimeWeb.WorkspaceChannel
  channel "team:*", RealtimeWeb.TeamChannel
  channel "user:*", RealtimeWeb.UserChannel
  channel "ticket:*", RealtimeWeb.TicketChannel

  @impl true
  def connect(params, socket, connect_info),
    do: RealtimeWeb.SocketAuth.connect(params, socket, connect_info, [:user, :api_key])

  @impl true
  def id(socket), do: "user_socket:" <> socket.assigns.sub
end
