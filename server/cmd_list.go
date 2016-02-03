package server

type ListArgs struct {
}

type ListResponse struct {
}

func (s *Server) List(args *ListArgs, reply *ListResponse) error {
	return nil
}
