package services

import (
	"context"
	e "errors"
	"reflect"

	"github.com/yaacov/tree-search-language/v6/pkg/tsl"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
)

//go:generate go tool -modfile=../../tools/go.mod mockgen -source=generic.go -package=services -destination=generic_mock.go

type GenericService interface {
	List(
		ctx context.Context, args *ListArguments, resourceList interface{},
	) (*api.PagingMeta, *errors.ServiceError)
}

func NewGenericService(genericDao dao.GenericDao) GenericService {
	return &sqlGenericService{genericDao: genericDao}
}

var _ GenericService = &sqlGenericService{}

type sqlGenericService struct {
	genericDao dao.GenericDao
}

// wrap all needed pieces for the LIST function
type listContext struct {
	ctx          context.Context
	resourceList interface{}
	args         *ListArguments
	pagingMeta   *api.PagingMeta
	resourceType string
}

func (s *sqlGenericService) newListContext(
	ctx context.Context, args *ListArguments, resourceList interface{},
) (*listContext, interface{}, *errors.ServiceError) {
	resourceModel := reflect.TypeOf(resourceList).Elem().Elem()
	if resourceModel.Kind() == reflect.Ptr {
		resourceModel = resourceModel.Elem()
	}
	resourceTypeStr := resourceModel.Name()
	if resourceTypeStr == "" {
		return nil, nil, errors.GeneralError("Could not determine resource type")
	}
	return &listContext{
		ctx:          ctx,
		args:         args,
		pagingMeta:   &api.PagingMeta{Page: args.Page},
		resourceList: resourceList,
		resourceType: resourceTypeStr,
	}, reflect.New(resourceModel).Interface(), nil
}

// List resourceList must be a pointer to a slice of database resource objects
func (s *sqlGenericService) List(
	ctx context.Context, args *ListArguments, resourceList interface{},
) (*api.PagingMeta, *errors.ServiceError) {
	listCtx, model, err := s.newListContext(ctx, args, resourceList)
	if err != nil {
		return nil, err
	}

	// the ordering for the sub functions matters.
	builders := []listBuilder{
		// build SQL to load related resource. for now, it delegates to gorm.preload.
		s.buildPreload,

		// add "ORDER BY"
		s.buildOrderBy,

		// scope to caller's tenancy before search filtering, so cross-tenant rows
		// never reach the search/pagination stage. no-op for system callers.
		s.buildTenantScope,

		// translate "search" into "WHERE"(s), and "JOIN"(s) if related resource is searched.
		s.buildSearch,

		// TODO: add any custom builder functions
	}

	d := s.genericDao.GetInstanceDao(ctx, model)

	// run all the "builders". they cumulatively add constructs to gorm by the context.
	// it stops when a builder function raises error or signals finished.
	var finished bool
	for _, builderFn := range builders {
		if finished, err = builderFn(listCtx, d); err != nil {
			return nil, err
		}
		if finished {
			if err = s.loadList(listCtx, d); err != nil {
				return nil, err
			}
			break
		}
	}
	return listCtx.pagingMeta, nil
}

/*** Define all sub functions in the type of listBuilder ***/
type listBuilder func(*listContext, dao.GenericDao) (finished bool, err *errors.ServiceError)

func (s *sqlGenericService) buildPreload(listCtx *listContext, d dao.GenericDao) (bool, *errors.ServiceError) {
	for _, preload := range listCtx.args.Preloads {
		d.Preload(preload)
	}
	return false, nil
}

func (s *sqlGenericService) buildOrderBy(listCtx *listContext, d dao.GenericDao) (bool, *errors.ServiceError) {
	if len(listCtx.args.Order) != 0 {
		cleanedOrderList, serviceErr := db.ArgsToOrder(listCtx.args.Order)
		if serviceErr != nil {
			return false, serviceErr
		}
		for _, orderArg := range cleanedOrderList {
			d.OrderBy(orderArg)
		}
	}
	return false, nil
}

// buildTenantScope applies the caller's tenancy filter to the list query.
// No-op for system callers or requests with no resolved tenant.
func (s *sqlGenericService) buildTenantScope(listCtx *listContext, d dao.GenericDao) (bool, *errors.ServiceError) {
	if clause, args := tenant.ScopeClause(listCtx.ctx); clause != "" {
		d.Where(dao.NewWhere(clause, args))
	}
	return false, nil
}

func (s *sqlGenericService) buildSearch(listCtx *listContext, d dao.GenericDao) (bool, *errors.ServiceError) {
	if listCtx.args.Search == "" {
		return true, nil
	}

	const maxSearchLength = 4096
	if len(listCtx.args.Search) > maxSearchLength {
		return false, errors.BadRequest(
			"search query exceeds maximum length of %d characters", maxSearchLength,
		)
	}

	tslTree, err := tsl.ParseTSL(listCtx.args.Search)
	if err != nil {
		return false, errors.BadRequest("failed to parse search query: %s", err.Error())
	}

	sql, values, serviceErr := db.TSLToSQL(tslTree, db.WalkConfig{
		TableName: d.GetTableName(),
	})
	if serviceErr != nil {
		return false, serviceErr
	}

	d.Where(dao.NewWhere(sql, values))
	return true, nil
}

func (s *sqlGenericService) loadList(listCtx *listContext, d dao.GenericDao) *errors.ServiceError {
	args := listCtx.args

	if countErr := d.Count(listCtx.resourceList, &listCtx.pagingMeta.Total); countErr != nil {
		switch {
		case db.IsDBConnectionError(countErr):
			return errors.ServiceUnavailable("Database connection unavailable")
		case db.IsInvalidColumnError(countErr):
			return errors.BadRequest("invalid field in search or order query")
		default:
			return errors.GeneralError("Unable to list resources: %s", countErr)
		}
	}

	// Set resourceList to be an empty slice with zero capacity. Real space will be allocated by g2.Find()
	if err := zeroSlice(listCtx.resourceList, 0); err != nil {
		return err
	}

	// gorm does not support Limit(0); also reject negative sizes defensively.
	if args.Size <= 0 {
		return nil
	}

	if err := d.Fetch(int((args.Page-1)*args.Size), int(args.Size), listCtx.resourceList); err != nil {
		switch {
		case e.Is(err, gorm.ErrRecordNotFound):
			listCtx.pagingMeta.Size = 0
		case db.IsDBConnectionError(err):
			return errors.ServiceUnavailable("Database connection unavailable")
		case db.IsInvalidColumnError(err):
			return errors.BadRequest("invalid field in search or order query")
		default:
			return errors.GeneralError("Unable to list resources: %s", err)
		}
	}
	listCtx.pagingMeta.Size = int64(reflect.ValueOf(listCtx.resourceList).Elem().Len())

	return nil
}

// Allocate a slice with size 'cap' of the type i
func zeroSlice(i interface{}, cap int64) *errors.ServiceError {
	v := reflect.ValueOf(i)
	if v.Kind() != reflect.Ptr {
		return errors.GeneralError("A non-pointer to a list of resources: %v", v.Type())
	}
	// get the value that the pointer v points to.
	v = v.Elem()
	if v.Kind() != reflect.Slice {
		return errors.GeneralError("A non-slice list of resources")
	}
	v.Set(reflect.MakeSlice(v.Type(), 0, int(cap)))
	return nil
}
